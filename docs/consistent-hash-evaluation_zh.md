# 一致性哈希评估

## 概述

本文档评估了 stream-metrics-route 中从基于 hashmod 的分片到 Jump Consistent Hash 的迁移，分析了技术原理、性能影响和迁移影响。

## 问题背景

原始实现使用 `hash % N` 在后端节点间分片指标。虽然简单，但这种方法有一个关键缺点：当从 N 扩展到 N+1 个后端时，**100% 的指标需要重新分片**，导致：

- 所有时间序列的完全重新分配
- 迁移过程中的负载不平衡
- 下游系统中聚合状态的破坏
- 不必要的网络流量和处理开销

这对于生产环境特别有问题，因为稳定路由对于指标聚合和去重至关重要。

## 评估的替代方案

| 算法 | 时间复杂度 | 内存使用 | 平衡质量 | 迁移成本 |
|------|------------|----------|----------|----------|
| hash % N (hashmod) | O(1) | O(1) | 良好 | 100% 重新分片 |
| 环状一致性哈希 | O(log N) | O(N) | 中等（需要虚拟节点） | ~K/N (K=键数) |
| Rendezvous (HRW) 哈希 | O(N) | O(1) | 良好 | ~K/N |
| Jump Consistent Hash | O(1) | O(1) | 优秀 | ~K/(N+1) |
| Maglev 哈希 | O(1) | O(N*M) | 良好 | 取决于表格 |

### 主要发现：

1. **Hashmod**：简单但迁移成本为 100%
2. **环状一致性哈希**：需要虚拟节点才能获得良好平衡，内存使用较高
3. **Rendezvous**：平衡性优秀但 O(N) 复杂度使其在大规模部署中性能较差
4. **Maglev**：性能良好但需要大型查找表
5. **Jump Consistent Hash**：在 O(1) 复杂度、最小迁移和优秀分布之间的最佳平衡

## 选择 Jump Consistent Hash 的理由

### 技术优势

1. **最小迁移**：当从 N 扩展到 N+1 个后端时，只有 ~1/(N+1) 的键需要重新映射
2. **O(1) 复杂度**：常量时间计算，无内存分配
3. **无依赖**：仅 15 行独立的自包含代码
4. **优秀分布**：在桶间具有均匀的概率分布
5. **无虚拟节点**：与环状算法不同，不需要配置调优

### 代码对比

**旧版本 (hashmod)：**
```go
func hashMod(mode int, hash uint32) int {
    if mode <= 1 {
        return 0
    }
    return int(hash % uint32(mode))
}
```

**新版本 (Jump Consistent Hash)：**
```go
func JumpConsistentHash(key uint64, numBuckets int) int {
    if numBuckets <= 1 {
        return 0
    }
    var b int64 = -1
    var j int64 = 0
    for j < int64(numBuckets) {
        b = j
        key = key*2862933555777941757 + 1
        j = int64(float64(b+1) * (float64(int64(1)<<31) / float64((key>>33)+1)))
    }
    return int(b)
}
```

## 迁移影响评估

### 破坏性变更

- **所有指标的 `stream_task_id` 值将会改变**：这是轻微的破坏性变更
- `stream_task_id` 是 vmagent 用于聚合去重的透明标签
- **具体值不重要** - 只在同一维度内保持一致即可

### 无需配置标志

- 在维护窗口期间进行无缝切换
- 静态 YAML 配置意味着节点变更本就需要重启
- 无需额外的配置管理复杂性

### 分布分析

**迁移前 (hashmod)**：从 100 个后端扩展到 101 个
- 100% 的指标重新映射到不同的 stream_task_id 值
- 所有节点间的完全重新分配

**迁移后 (Jump Hash)**：从 100 个后端扩展到 101 个
- 只有 ~1/101 的指标需要重新映射（约 0.99% 的键）
- 99.01% 的指标保持其 stream_task_id 值不变

### 功能保持

- `filterLabels` 功能完全保持不变
- `SortLabelsHashKey` 中的 FNV-32a 哈希计算保持不变
- 除最终哈希函数外，所有路由逻辑保持相同
- 熔断器和重试逻辑不受影响

## 性能分析

### 实现特点

- **Jump Hash**：约 15 行代码，无内存分配，无查找表
- **O(1) 时间复杂度**：与 hashmod 相同，但具有最小迁移特性
- **分布**：在桶间均匀分布（每个桶获得约 1/N 的键）

### 基准测试考虑

- 与 hashmod 相比没有额外的 CPU 开销
- 内存使用保持恒定（O(1)）
- 无需缓存或维护的查找表
- 确定性行为 - 相同输入总是产生相同输出

### 生产环境优势

1. **优雅扩展**：增量添加后端不会导致完全重新平衡
2. **稳定聚合**：在扩展操作期间保持 vmagent 聚合状态
3. **减少负载**：迁移期间最小化网络流量和处理
4. **可预测行为**：分布质量的数学保证

## 理论基础

Jump Consistent Hash 算法基于学术论文：

> "A Fast, Minimal Memory, Consistent Hash Algorithm" by John Lamping and Eric Veach, Google, 2014
> http://arxiv.org/abs/1406.2294

这篇论文为该算法提供了数学基础，证明了其最小迁移特性和均匀分布特性。

## 实际实现

`pkg/remote/remotecluster.go` 中的生产实现现在使用：

```go
hash := common.SortLabelsHashKey(ts.Labels)
dime := common.JumpConsistentHash(uint64(hash), r.dimension)  // stream_task_id
hashnode = common.SortLabelsHashKey(tmpLabels)
tmpch := common.JumpConsistentHash(uint64(hashnode), r.uplen)  // 节点选择
```

这保持了双哈希架构，同时将 hashmod 函数替换为 Jump Consistent Hash，用于任务分配和节点选择。

## 参考资料

1. [带有实现细节的博客文章](https://blog.mickeyzzc.tech/posts/telemetry/stream-metrics-one/)
2. [Jump Consistent Hash 论文](http://arxiv.org/abs/1406.2294)
3. [原始 hashmod 实现](https://github.com/mickeyzzc/stream-metrics-route/blob/v0.1.2/pkg/common/relabel.go#L83)
4. [Jump Consistent Hash 实现](https://github.com/mickeyzzc/stream-metrics-route/blob/main/pkg/common/relabel.go#L83)