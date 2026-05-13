# 架构设计

## 概述

stream-metrics-route 是一个高性能指标路由网关，旨在解决大规模 Prometheus/VictoriaMetrics 部署中的高基数指标分发问题。

## 问题背景

在大规模 Prometheus 高吞吐场景中，常见挑战包括：

1. **高基数指标**：如 `istio_request_total` 等服务可能产生数百万个独特时间序列
2. **一致性路由需求**：相同维度指标必须路由到同一处理节点才能准确聚合
3. **后端多样性**：不同指标可能需要路由到不同后端（vmagent、Kafka 等）
4. **可靠性**：后端故障时需要熔断器和重试机制

## 解决方案架构

```mermaid
flowchart TB
    subgraph 客户端
        P[Prometheus]
        AG[Prometheus Agent]
        APP[应用 SDK]
    end

    subgraph 网关
        direction TB
        HTTP[HTTP 处理器<br/>Snappy 解码]
        VALIDATE[验证<br/>大小限制]
        RELABEL[Relabel 过滤]
        ROUTE[路由器<br/>双重 Hashmod]
    end

    subgraph 后端
        subgraph RemoteWrite
            VM0[vmagent-0]
            VM1[vmagent-1]
            VM2[vmagent-2]
            VMS[(VictoriaMetrics<br/>集群)]
        end
        subgraph Kafka
            KAFKA[Kafka 集群]
            CONS[消费者]
        end
    end

    P -->|Remote Write| HTTP
    AG -->|Remote Write| HTTP
    APP -->|Remote Write| HTTP

    HTTP --> VALIDATE
    VALIDATE --> RELABEL
    RELABEL --> ROUTE

    ROUTE -->|stream_task_id=0| VM0
    ROUTE -->|stream_task_id=1| VM1
    ROUTE -->|stream_task_id=2| VM2
    ROUTE -->|过滤后的指标| KAFKA

    VM0 --> VMS
    VM1 --> VMS
    VM2 --> VMS
    KAFKA --> CONS
```

## 核心组件

### 1. HTTP 处理器 (`pkg/receive/`)

处理传入的 Prometheus Remote Write 请求：

- **Snappy 解压缩**：解码 snappy 压缩的 protobuf 数据
- **请求验证**：强制执行大小限制和超时
- **指标追踪**：记录时间序列和样本数量

### 2. 路由器 (`pkg/router/`)

实现带双重 hashmod 的路由逻辑：

```mermaid
flowchart TD
    A[输入 TimeSeries] --> B[应用 Relabel 规则]
    B --> C{是否有规则匹配?}
    C -->|否| D[丢弃序列]
    C -->|是| E{多个路由器?}
    E -->|否| F[单一路由器]
    E -->|是| G[并行路由]
    F --> H[单一存储调用]
    G --> I[WaitGroup + 错误收集]
    I --> J{聚合结果}
    J --> K{有错误?}
    K -->|否| L[返回 200 OK]
    K -->|是| M[返回错误详情]
```

### 3. 远程集群客户端 (`pkg/remote/`)

管理到多个 Remote Write 端点的连接：

- **Hashmod 路由**：基于指标标签的一致性哈希
- **熔断器**：防止级联故障
- **重试逻辑**：对临时故障进行指数退避重试

### 4. 熔断器 (`pkg/remote/circuitbreaker.go`)

实现熔断器模式：

```mermaid
stateDiagram-v2
    [*] --> Closed
    Closed --> Open: 达到失败阈值
    Open --> HalfOpen: 超时结束
    HalfOpen --> Closed: 达到成功阈值
    HalfOpen --> Open: 任何失败
    Closed --> Closed: 成功重置计数器
    Open --> Open: 仍在超时中
```

## 双重 Hashmod 算法

双重 hashmod 算法解决确保相同指标发送到同一处理节点的问题：

### 第一步：任务 ID 分配

```go
// 计算所有标签的哈希
hash := sortLabelsHashKey(ts.Labels)

// 第一次 hashmod：分配任务分区 ID
dime := hashMod(r.dimension, hash)  // dimension 通常 = 100

// 注入 stream_task_id 标签
ts.Labels = append(ts.Labels, prompb.Label{
    Name:  "stream_task_id",
    Value: fmt.Sprintf("%d", dime),
})
```

### 第二步：节点选择

```go
// 仅使用过滤标签计算哈希
hashnode := sortLabelsHashKey(filterLabels)

// 第二次 hashmod：选择后端节点
tmpch := hashMod(r.uplen, hashnode)

// 路由到特定后端
sendSamplesChan[tmpch] = append(sendSamplesChan[tmpch], ts)
```

### 为什么需要双重 Hashmod？

| 场景 | 单一 Hashmod | 双重 Hashmod |
|------|-------------|--------------|
| 相同指标，不同实例 | 路由到不同节点 | 路由到同一节点（按 task_id） |
| 节点数量变化 | 所有指标重新分配 | 仅影响路由，不影响任务分配 |
| 任务扩容 | 所有指标重新路由 | 仅新任务边界受影响 |

## 数据流

```mermaid
sequenceDiagram
    participant C as Prometheus
    participant G as 网关
    participant R as 路由器
    participant RC as 远程集群
    participant CB as 熔断器
    participant RW as RemoteWriter

    C->>G: Remote Write 请求
    G->>G: 验证和解码
    G->>R: 过滤后的 TimeSeries
    R->>R: 应用 Relabel 规则
    R->>RC: 按 Hashmod 分组
    RC->>CB: 允许请求?
    CB-->>RC: 允许
    RC->>RW: 存储请求
    RW->>RW: 退避重试
    RW-->>RC: 成功/失败
    RC-->>R: 结果
    R-->>G: 聚合结果
    G-->>C: HTTP 响应
```

## 可靠性特性

### 1. 请求大小限制

```go
const maxRequestSize = 100 * 1024 * 1024 // 100MB

limitedReader := io.LimitReader(c.Request.Body, maxRequestSize)
```

### 2. 写入超时

```go
ctx, cancel := context.WithTimeout(c.Request.Context(), 30*time.Second)
defer cancel()
router.Store(ctx, req.Timeseries)
```

### 3. 熔断器

| 参数 | 默认值 | 描述 |
|------|--------|------|
| 失败阈值 | 5 | 连续 5 次失败后打开熔断器 |
| 成功阈值 | 3 | 半开状态下 3 次成功后关闭熔断器 |
| 超时时间 | 30s | 尝试关闭熔断器前等待的时间 |

### 4. 指数退避重试

```go
for attempt := 0; attempt <= maxRetries; attempt++ {
    if attempt > 0 {
        backoff = time.Duration(math.Min(float64(backoff*2), float64(maxBackoff)))
        time.Sleep(backoff)
    }
    err := store(ctx, data)
    if err == nil {
        return nil
    }
}
```

## 性能考虑

### 并行处理

- 路由器使用 goroutine 进行并行后端写入
- WaitGroup 确保所有写入完成后再返回
- 错误被收集并报告

### 内存效率

- Relabel 过滤器在早期减少不必要的数据
- Kafka 写入的批处理
- HTTP 客户端的连接池

### 监控指标

用于性能调优的关键指标：

- `stream_router_write_duration_seconds`：每个路由器的延迟
- `stream_remote_write_timeseries_total`：吞吐量
- `stream_remote_write_failures_total`：错误率
