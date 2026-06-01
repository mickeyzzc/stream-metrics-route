# Stream Metrics Route - Code Wiki

## 目录
1. [项目概述](#项目概述)
2. [架构设计](#架构设计)
3. [核心模块](#核心模块)
4. [关键类与函数](#关键类与函数)
5. [依赖关系](#依赖关系)
6. [配置与运行](#配置与运行)
7. [API 接口](#api-接口)
8. [测试与部署](#测试与部署)

---

## 项目概述

### 项目简介
**Stream Metrics Route** 是一个高性能的指标路由网关，专门用于解决大规模 Prometheus/VictoriaMetrics 部署中的高基数指标分发问题。

### 核心特性
- **Jump Consistent Hash 调度**: 确保同维度指标一致路由到同一后端节点
- **Prometheus Remote Write**: 原生支持 Prometheus Remote Write 协议
- **Kafka 集成**: 可选的 Kafka 生产者实现异步消息分发
- **熔断器模式**: 内置熔断器，防止级联故障
- **Relabel 规则**: 完整支持 Prometheus Relabeling 规则
- **Prometheus 指标**: 全面的监控指标，便于可观测性

### 适用场景
- 大规模 Prometheus 集群指标分发
- 高基数指标分片处理
- 多后端（RemoteWrite/Kafka）灵活路由
- 高可用性要求的监控架构

---

## 架构设计

### 整体架构
```
┌─────────────────┐
│  客户端层        │
│  Prometheus     │
│  Prometheus Agent│
│  应用 SDK       │
└────────┬────────┘
         │ Remote Write
         ▼
┌─────────────────────────────────────┐
│         网关层                       │
│  ┌───────────────────────────────┐  │
│  │  HTTP 接收层 (receive)        │  │
│  │  - Snappy 解码                │  │
│  │  - 请求验证                   │  │
│  └───────────────┬───────────────┘  │
│                  │                  │
│  ┌───────────────▼───────────────┐  │
│  │  路由层 (router)              │  │
│  │  - Relabel 过滤               │  │
│  │  - Jump Consistent Hash 调度  │  │
│  └───────────────┬───────────────┘  │
└──────────────────┼──────────────────┘
                   │
        ┌──────────┴──────────┐
        │                     │
        ▼                     ▼
┌───────────────┐    ┌───────────────┐
│ Remote Write  │    │    Kafka      │
│  后端层       │    │  后端层       │
│ (remote)      │    │ (kafkaclient) │
└───────────────┘    └───────────────┘
```

### 数据流程

#### 1. 请求接收
- HTTP 服务器接收 Prometheus Remote Write 请求
- 验证请求大小和超时
- Snappy 解压缩 + Protobuf 解码

#### 2. 指标过滤
- 应用 Relabel 规则过滤指标
- 丢弃不需要的时间序列

#### 3. 路由分配
- 对指标标签进行排序哈希
- 使用 **Jump Consistent Hash** 算法分配分区
- 注入 `stream_task_id` 标签
- 二次哈希选择后端节点

#### 4. 数据转发
- 并行发送到多个后端
- 熔断器保护
- 指数退避重试

### 核心算法：Double Jump Consistent Hash

#### 为什么需要双重哈希？
1. **任务稳定性**: 第一层哈希分配 `stream_task_id`，保证相同指标始终属于同一任务
2. **节点灵活性**: 第二层哈希选择后端，节点扩缩容不影响任务分配

#### 实现原理
```go
// 第一步：分配任务 ID（全局稳定）
hash := sortLabelsHashKey(ts.Labels)
taskID := JumpConsistentHash(uint64(hash), dimension)
ts.Labels = append(ts.Labels, prompb.Label{
    Name:  "stream_task_id",
    Value: fmt.Sprintf("%d", taskID),
})

// 第二步：选择后端节点（可变化）
hashnode := sortLabelsHashKey(filterLabels)
nodeIndex := JumpConsistentHash(uint64(hashnode), uplen)
```

---

## 核心模块

### 1. cmd/stream-metrics-route - 程序入口

#### 功能
- 命令行参数解析
- 配置加载
- HTTP 服务初始化
- 路由构建
- 信号处理与优雅关闭

#### 主要组件
- **main.go**: 主程序文件
  - 配置加载：`setting.LoadFile()`
  - 路由构建：`router.BuildRouters()`
  - HTTP 服务启动：`gin.Default()`

---

### 2. pkg/router - 路由核心

#### 功能
- 多路由规则管理
- Relabel 规则应用
- 并行后端调用
- 结果聚合与错误处理

#### 核心结构
```go
// Routers - 路由器集合
type Routers struct {
    Routers map[string]*Router
    lock    sync.RWMutex
}

// Router - 单个路由器
type Router struct {
    Name                 string
    MetricRelabelConfigs []*relabel.Config
    RemoteStore          RemoteStore
}
```

#### 关键方法
| 方法 | 说明 |
|------|------|
| `BuildRouters()` | 根据配置构建路由器 |
| `Store()` | 并行存储指标到所有路由器 |
| `filterLabels()` | 应用 Relabel 规则过滤指标 |
| `GetRouterStats()` | 获取路由统计信息 |

---

### 3. pkg/remote - Remote Write 客户端

#### 功能
- 多个 Remote Write 后端管理
- Jump Consistent Hash 路由
- 熔断器保护
- 指数退避重试
- 统计信息收集

#### 核心结构
```go
// RemoteCluster - 远程集群管理器
type RemoteCluster struct {
    uplen          int
    dimension      int
    filterLabels   []string
    Writers        map[int]*RemoteWriterUrl
    Name           string
    circuitBreaker *CircuitBreaker
    stats          RemoteClusterStats
}
```

---

### 4. pkg/kafkaclient - Kafka 生产者

#### 功能
- Kafka 生产者管理
- 异步/同步发送模式
- 消息压缩
- 批量发送
- 重试机制

#### 核心结构
```go
// KafkaClient - Kafka 客户端
type KafkaClient struct {
    name          string
    match         map[string]*dto.MetricFamily
    compression   compress.Compression
    Producer      *kafka.Writer
    TopicTemplate template.Template
    config        kafka.WriterConfig
    stats         KafkaStats
}
```

---

### 5. pkg/receive - HTTP 接收层

#### 功能
- HTTP 请求处理
- 请求大小限制
- Snappy 解压缩
- Protobuf 解码
- 超时控制
- 错误处理

#### 核心结构
```go
// Receive - HTTP 接收处理器
type Receive struct {
    Upstream map[int]*remote.RemoteWriterUrl
    uplen    int
    MaxSize  int64
    Timeout  time.Duration
}
```

---

### 6. pkg/setting - 配置管理

#### 功能
- YAML 配置文件加载
- 配置结构定义
- 配置验证

#### 核心配置结构
```go
// Config - 主配置
type Config struct {
    GlobalConfig GlobalConf
    RouterRule   []RouterRuleConf
}

// RouterRuleConf - 路由规则配置
type RouterRuleConf struct {
    RouterName         string
    UpStreams          UpStreamsConf
    HashLabels         HashLabels
    MetricRelabelConfigs []*relabel.Config
}
```

---

### 7. pkg/common - 通用工具

#### 功能
- Jump Consistent Hash 算法实现
- 标签排序与哈希
- 标签快速编解码

#### 关键函数
| 函数 | 说明 |
|------|------|
| `JumpConsistentHash()` | Jump Consistent Hash 算法 |
| `SortLabelsHashKey()` | 标签排序并计算哈希 |

---

### 8. pkg/telemetry - 可观测性

#### 功能
- 日志系统
- Prometheus 指标
- 链路追踪

---

## 关键类与函数

### 入口函数 (cmd/stream-metrics-route/main.go)

#### `init()` - 初始化
```go
func init() {
    flag.Parse()
    configFile = *confPath + "/" + *confName
    defaultCfg, err = setting.LoadFile(configFile)
    // 初始化路由和指标
    router.BuildRouters(defaultCfg)
}
```

#### `main()` - 主函数
```go
func main() {
    // 初始化接收处理器
    receiver := receive.NewReceive(*maxRequestSize, *writeTimeout)
    
    // 设置 HTTP 路由
    router_v1 := route.Group("api/v1")
    router_v1.POST("write", receiver.Handler())
    router_v1.POST("receive", receiver.Handler())
    
    // 启动服务
    srv := &http.Server{Addr: ":" + *listenPort}
    go srv.ListenAndServe()
    
    // 信号处理
    // ...
}
```

---

### 路由器 (pkg/router/router.go)

#### `BuildRouters()` - 构建路由器
```go
func BuildRouters(cfg *setting.Config) {
    // 根据配置创建路由器
    // 支持 RemoteWriter 和 Kafka 两种后端类型
}
```

#### `Store()` - 存储指标
```go
func (rs *Routers) Store(ctx context.Context, req []prompb.TimeSeries) []StoreResult {
    // 并行调用所有路由器的 Store 方法
    // 使用 WaitGroup 等待完成
    // 收集并返回结果
}
```

#### `filterLabels()` - 应用 Relabel 规则
```go
func (r *Router) filterLabels(ts []prompb.TimeSeries) []prompb.TimeSeries {
    // 对每个时间序列应用 Relabel 规则
    // 返回保留的时间序列
}
```

---

### 远程集群 (pkg/remote/remotecluster.go)

#### `NewRemoteCluster()` - 创建远程集群
```go
func NewRemoteCluster(name string, dimension int, filterLabels []string, Urls []string) *RemoteCluster {
    // 初始化熔断器
    // 创建 RemoteWriterUrl 实例
    // 返回 RemoteCluster
}
```

#### `Store()` - 发送指标到远程
```go
func (r *RemoteCluster) Store(ctx context.Context, req []prompb.TimeSeries) error {
    // 检查熔断器状态
    // 使用 Jump Consistent Hash 分配指标
    // 并行发送到所有后端
    // 记录成功/失败
}
```

---

### 熔断器 (pkg/remote/circuitbreaker.go)

#### `NewCircuitBreaker()` - 创建熔断器
```go
func NewCircuitBreaker(failureThreshold, successThreshold int, timeout time.Duration) *CircuitBreaker {
    // 初始化熔断器，默认状态为 Closed
}
```

#### 状态转换
| 当前状态 | 事件 | 下一状态 |
|---------|------|---------|
| Closed | 连续失败达到阈值 | Open |
| Open | 超时结束 | HalfOpen |
| HalfOpen | 成功达到阈值 | Closed |
| HalfOpen | 任何失败 | Open |

---

### Kafka 客户端 (pkg/kafkaclient/kafka.go)

#### `NewKafka()` - 创建 Kafka 客户端
```go
func NewKafka(name string, cfg setting.KafkaConfig) (*KafkaClient, error) {
    // 解析主题模板
    // 配置 Kafka Writer
    // 设置压缩
}
```

#### `Store()` - 发送到 Kafka
```go
func (k *KafkaClient) Store(ctx context.Context, req []prompb.TimeSeries) error {
    // 处理指标数据
    // 分批发送到 Kafka
    // 指数退避重试
}
```

---

### 通用工具 (pkg/common/relabel.go)

#### `JumpConsistentHash()` - Jump Consistent Hash 算法
```go
func JumpConsistentHash(key uint64, numBuckets int) int {
    // 实现 Lamping & Veach (2014) 算法
    // 时间复杂度 O(1)，空间复杂度 O(0)
    // 最小化重映射
}
```

**算法特点**：
- 一致性：相同 key 始终映射到相同 bucket
- 最小化重映射：bucket 数量从 N 变为 N+1 时，只有约 1/(N+1) 的 key 需要重映射
- 高性能：无需预计算，无需额外内存

---

## 依赖关系

### Go 模块依赖
| 模块 | 版本 | 用途 |
|------|------|------|
| github.com/VictoriaMetrics/VictoriaMetrics | v1.93.14 | 标签编解码优化 |
| github.com/gin-gonic/gin | v1.9.1 | HTTP Web 框架 |
| github.com/gogo/protobuf | v1.3.2 | Protobuf 编解码 |
| github.com/golang/snappy | v0.0.4 | Snappy 压缩 |
| github.com/prometheus/client_golang | v1.19.0 | Prometheus 指标 |
| github.com/prometheus/prometheus | v0.51.2 | Prometheus 库 |
| github.com/segmentio/kafka-go | v0.4.47 | Kafka 客户端 |
| gopkg.in/yaml.v2 | v2.4.0 | YAML 配置解析 |

### 模块间依赖关系
```
cmd/stream-metrics-route
  ├── pkg/receive
  │     ├── pkg/router
  │     │     ├── pkg/remote
  │     │     ├── pkg/kafkaclient
  │     │     ├── pkg/setting
  │     │     └── pkg/telemetry
  │     ├── pkg/setting
  │     └── pkg/telemetry
  ├── pkg/router
  ├── pkg/setting
  └── pkg/telemetry

pkg/remote
  └── pkg/common

pkg/kafkaclient
  └── pkg/setting
```

---

## 配置与运行

### 配置文件 (config.yaml)

#### 示例配置
```yaml
router_rules:
  # 路由到 VictoriaMetrics 集群
  - router_name: "route-to-vm"
    hash_labels:
      mode: 100  # 任务维度
      labels:
        - "__name__"
        - "job"
    upstreams:
      upstream_type: "remotewriter"
      upstream_urls:
        - "http://vmagent-0:8429/api/v1/write"
        - "http://vmagent-1:8429/api/v1/write"
        - "http://vmagent-2:8429/api/v1/write"
  
  # 路由到 Kafka
  - router_name: "route-to-kafka"
    upstreams:
      upstream_type: "kafka"
      kafka_config:
        kafka_broker_list: "kafka-0:9092,kafka-1:9092"
        kafka_topic: "metrics"
        kafka_compression: "snappy"
        kafka_batch_num_messages: 1000
        async: false
```

#### 配置参数详解

##### GlobalConfig
| 参数 | 类型 | 默认 | 说明 |
|------|------|------|------|
| prefix | string | "stream-metrics-route" | 指标前缀 |

##### RouterRuleConf
| 参数 | 类型 | 必须 | 说明 |
|------|------|------|------|
| router_name | string | 是 | 路由名称 |
| upstreams | UpStreamsConf | 是 | 上游配置 |
| hash_labels | HashLabels | 否 | 哈希标签配置 |
| metric_relabel_configs | []relabel.Config | 否 | Relabel 规则 |

##### UpStreamsConf
| 参数 | 类型 | 必须 | 说明 |
|------|------|------|------|
| upstream_type | string | 是 | "remotewriter" 或 "kafka" |
| upstream_urls | []string | 否 | Remote Write URL 列表 |
| kafka_config | KafkaConfig | 否 | Kafka 配置 |

##### KafkaConfig
| 参数 | 类型 | 默认 | 说明 |
|------|------|------|------|
| kafka_broker_list | string | - | Kafka Broker 列表 |
| kafka_topic | string | - | Topic 名称 |
| kafka_compression | string | "none" | 压缩算法 |
| kafka_batch_num_messages | int | 1000 | 批量消息数 |
| kafka_batch_bytes | int | 1048576 | 批量字节数 |
| async | bool | false | 是否异步发送 |
| balancer | string | "leastbytes" | 分区均衡器 |

### 命令行参数
| 参数 | 默认 | 说明 |
|------|------|------|
| --config.path | 当前目录 | 配置文件路径 |
| --config.name | "config.yaml" | 配置文件名 |
| --log.level | "info" | 日志级别 (debug/info) |
| --listen.port | "8080" | 监听端口 |
| --max.request.size | 100MB | 最大请求大小 (字节) |
| --write.timeout | 30s | 写入超时 |
| --pprof.enabled | true | 启用 pprof |

### 运行方式

#### 1. 源码运行
```bash
# 克隆项目
git clone https://github.com/mickeyzzc/stream-metrics-route.git
cd stream-metrics-route

# 下载依赖
go mod tidy

# 编译
go build -o stream-metrics-route ./cmd/stream-metrics-route

# 运行
./stream-metrics-route --config.path=/path/to/config
```

#### 2. Docker 运行
```bash
# 构建镜像
docker build -t stream-metrics-route:latest .

# 运行容器
docker run -d \
  -p 8080:8080 \
  -v /path/to/config.yaml:/app/config.yaml \
  stream-metrics-route:latest
```

#### 3. Kubernetes 部署
```bash
kubectl apply -f docs/deploy/kubernetes.yaml
```

---

## API 接口

### 1. Prometheus Remote Write
- **URL**: `/api/v1/write` 或 `/api/v1/receive`
- **方法**: POST
- **Content-Type**: `application/x-protobuf`
- **Content-Encoding**: `snappy`
- **描述**: 接收 Prometheus Remote Write 请求

#### 请求示例 (Prometheus 配置)
```yaml
remote_write:
  - url: "http://stream-metrics-route:8080/api/v1/write"
```

#### 响应
- **200 OK**: 成功
- **202 Accepted**: 部分失败
- **400 Bad Request**: 请求错误
- **413 Request Entity Too Large**: 请求过大
- **503 Service Unavailable**: 所有后端不可用

### 2. Prometheus 指标
- **URL**: `/metrics`
- **方法**: GET
- **描述**: 暴露 Prometheus 格式的指标

#### 关键指标
| 指标名称 | 类型 | 描述 |
|----------|------|------|
| `stream_receive_duration_seconds` | Histogram | 请求处理耗时 |
| `stream_receive_series_data_total` | Counter | 接收的时间序列数 |
| `stream_receive_samples_data_total` | Counter | 接收的样本数 |
| `stream_router_timeseries_total` | Counter | 路由的时间序列数 |
| `stream_router_write_duration_seconds` | Histogram | 路由器写入耗时 |
| `stream_router_errors_total` | Counter | 路由器错误数 |
| `stream_remote_write_timeseries_total` | Counter | Remote Write 发送数 |
| `stream_remote_write_failures_total` | Counter | Remote Write 失败数 |
| `stream_kafka_objects_written_total` | Counter | Kafka 写入对象数 |
| `stream_kafka_objects_failed_total` | Counter | Kafka 失败对象数 |

### 3. 统计信息
- **URL**: `/stats`
- **方法**: GET
- **描述**: 获取路由统计信息

#### 响应示例
```json
{
  "code": 2000,
  "msg": "ok",
  "data": {
    "route-to-vm": {
      "name": "route-to-vm",
      "state": "closed",
      "total_requests": 12345,
      "success_count": 12300,
      "failure_count": 45,
      "circuit_breaker": {
        "state": "closed",
        "failures": 0,
        "successes": 0
      }
    },
    "route-to-kafka": {
      "name": "route-to-kafka",
      "success_count": 12345,
      "failure_count": 0
    }
  }
}
```

### 4. 健康检查
- **URL**: `/-/health`
- **方法**: GET
- **描述**: 健康检查端点

### 5. 就绪检查
- **URL**: `/-/ready`
- **方法**: GET
- **描述**: 就绪检查端点

### 6. PProf (调试)
- **URL**: `/debug/pprof/*`
- **方法**: GET
- **描述**: Go PProf 调试端点

---

## 测试与部署

### 测试
```bash
# 运行所有测试
make test

# 或直接使用 go test
go test ./... -v
```

### 代码检查
```bash
# Go Vet
make lint

# 格式化
make fmt
```

### 构建
```bash
# 编译 Linux AMD64 二进制文件
make build

# 构建 Docker 镜像
make docker
```

### 部署建议

#### 高可用性部署
- 部署多个 stream-metrics-route 实例
- 使用负载均衡器分发流量
- 配置适当的资源限制

#### 监控
- 收集 `/metrics` 端点的 Prometheus 指标
- 监控关键指标：错误率、延迟、吞吐量
- 设置告警规则

#### 容量规划
- 根据预期 QPS 配置实例数量
- 监控内存和 CPU 使用
- 合理配置批量大小和超时

---

## 扩展与开发

### 添加新的后端类型
1. 在 `pkg/setting/config.go` 中添加新的 `RemoteType`
2. 在 `pkg/router/router.go` 的 `BuildRouters()` 中添加处理逻辑
3. 实现 `RemoteStore` 接口
4. 更新文档和配置示例

### 自定义 Relabel 规则
使用标准 Prometheus Relabel 规则语法：
```yaml
metric_relabel_configs:
  - source_labels: [__name__]
    regex: "go_.*"
    action: keep
```

---

## 参考资料

- [Jump Consistent Hash 论文](https://arxiv.org/abs/1406.2294)
- [Prometheus Remote Write 规范](https://prometheus.io/docs/prometheus/latest/storage/#remote-storage-integrations)
- [VictoriaMetrics 文档](https://docs.victoriametrics.com/)
- [Gin Web Framework](https://gin-gonic.com/)

---

## 版本历史

请参考 [CHANGELOG.md](./CHANGELOG.md)

---

## 许可证

请参考 [LICENSE](./LICENSE)
