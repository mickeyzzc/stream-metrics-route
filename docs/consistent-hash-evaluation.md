# Consistent Hash Evaluation

## Overview

This document evaluates the migration from hashmod-based sharding to Jump Consistent Hash in stream-metrics-route, analyzing the technical rationale, performance implications, and migration impact.

## Problem Statement

The original implementation used `hash % N` for sharding metrics across backend nodes. While simple, this approach has a critical drawback: when scaling from N to N+1 backends, **100% of metrics are reshuffled**, causing:

- Complete redistribution of all time series
- Potential load imbalance during migration
- Disrupted aggregation state in downstream systems
- Unnecessary network traffic and processing overhead

This is particularly problematic for production environments where stable routing is essential for metrics aggregation and deduplication.

## Evaluated Alternatives

| Algorithm | Time Complexity | Memory | Balance Quality | Migration Cost |
|-----------|----------------|--------|-----------------|----------------|
| hash % N (hashmod) | O(1) | O(1) | Good | 100% reshuffle |
| Ring-based Consistent Hash | O(log N) | O(N) | Moderate (needs vnodes) | ~K/N (K=keys) |
| Rendezvous (HRW) Hash | O(N) | O(1) | Good | ~K/N |
| Jump Consistent Hash | O(1) | O(1) | Excellent | ~K/(N+1) |
| Maglev Hash | O(1) | O(N*M) | Good | Depends on table |

### Key Findings:

1. **Hashmod**: Simple but has 100% migration cost
2. **Ring-based**: Requires virtual nodes for good balance, higher memory usage
3. **Rendezvous**: Excellent balance but O(N) complexity makes it slow for large deployments
4. **Maglev**: Good performance but requires large lookup tables
5. **Jump Consistent Hash**: Optimal balance of O(1) complexity, minimal migration, and excellent distribution

## Rationale for Jump Consistent Hash

### Technical Advantages

1. **Minimal Migration**: When scaling from N to N+1 backends, only ~1/(N+1) of keys remap
2. **O(1) Complexity**: Constant time computation with no memory allocation
3. **No Dependencies**: Only 15 lines of self-contained code
4. **Excellent Distribution**: Uniform probability distribution across buckets
5. **No Virtual Nodes**: Unlike ring-based algorithms, doesn't require configuration tuning

### Code Comparison

**Old (hashmod):**
```go
func hashMod(mode int, hash uint32) int {
    if mode <= 1 {
        return 0
    }
    return int(hash % uint32(mode))
}
```

**New (Jump Consistent Hash):**
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

## Migration Impact Assessment

### Breaking Changes

- **`stream_task_id` values will change for all metrics**: This is a soft breaking change
- The `stream_task_id` is an opaque label used by vmagent for aggregation deduplication
- **Exact values don't matter** - only consistency within the same dimension

### No Config Flag Needed

- Clean cutover during maintenance window
- Static YAML config means node changes require restart anyway
- No additional complexity for configuration management

### Distribution Analysis

**Before (hashmod)**: Scaling from 100 to 101 backends
- 100% of metrics remap to different stream_task_id values
- Complete redistribution across all nodes

**After (Jump Hash)**: Scaling from 100 to 101 backends  
- Only ~1/101 of metrics remap (~0.99% of keys)
- 99.01% of metrics maintain their stream_task_id values

### Feature Preservation

- `filterLabels` feature is preserved exactly as-is
- The FNV-32a hash computation in `SortLabelsHashKey` is unchanged
- All routing logic remains identical except for the final hash function
- Circuit breaker and retry logic unaffected

## Performance Analysis

### Implementation Characteristics

- **Jump Hash**: ~15 lines of code, no memory allocation, no lookup table
- **O(1) time complexity**: Same as hashmod but with minimal migration property
- **Distribution**: Uniform across buckets (each bucket gets ~1/N of keys)

### Benchmark Considerations

- No additional CPU overhead compared to hashmod
- Memory usage remains constant (O(1))
- No lookup tables to cache or maintain
- Deterministic behavior - same input always produces same output

### Production Benefits

1. **Graceful Scaling**: Incremental addition of backends doesn't cause complete rebalancing
2. **Stable Aggregation**: vmagent aggregation state preserved during scaling operations
3. **Reduced Load**: Minimal network traffic and processing during migrations
4. **Predictable Behavior**: Mathematical guarantees on distribution quality

## Theoretical Foundation

The Jump Consistent Hash algorithm is based on the academic paper:

> "A Fast, Minimal Memory, Consistent Hash Algorithm" by John Lamping and Eric Veach, Google, 2014
> http://arxiv.org/abs/1406.2294

This paper provides the mathematical foundation for the algorithm, proving its minimal migration property and uniform distribution characteristics.

## Real-world Implementation

The production implementation in `pkg/remote/remotecluster.go` now uses:

```go
hash := common.SortLabelsHashKey(ts.Labels)
dime := common.JumpConsistentHash(uint64(hash), r.dimension)  // stream_task_id
hashnode = common.SortLabelsHashKey(tmpLabels)
tmpch := common.JumpConsistentHash(uint64(hashnode), r.uplen)  // node selection
```

This maintains the dual-hash architecture while replacing the hashmod function with Jump Consistent Hash for both task assignment and node selection.

## References

1. [Blog post with implementation details](https://blog.mickeyzzc.tech/posts/telemetry/stream-metrics-one/)
2. [Jump Consistent Hash paper](http://arxiv.org/abs/1406.2294)
3. [Original hashmod implementation](https://github.com/mickeyzzc/stream-metrics-route/blob/v0.1.2/pkg/common/relabel.go#L83)
4. [Jump Consistent Hash implementation](https://github.com/mickeyzzc/stream-metrics-route/blob/main/pkg/common/relabel.go#L83)