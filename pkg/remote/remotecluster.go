package remote

import (
	"context"
	"fmt"
	"sync"
	"time"

	"stream-metrics-route/pkg/common"

	"github.com/prometheus/prometheus/prompb"
)

type RemoteCluster struct {
	uplen        int
	dimension    int
	filterLabels []string
	Writers      map[int]*RemoteWriterUrl
	Name         string

	circuitBreaker *CircuitBreaker
	mu             sync.RWMutex
	stats         RemoteClusterStats
}

type RemoteClusterStats struct {
	TotalRequests   int64
	SuccessCount   int64
	FailureCount   int64
	LastRequestAt  time.Time
	LastSuccessAt  time.Time
	LastFailureAt  time.Time
	AvgLatency     float64
}

func NewRemoteCluster(name string, dimension int, filterLabels []string, Urls []string) *RemoteCluster {
	cb := NewCircuitBreaker(
		5,
		3,
		30*time.Second,
	)

	writers := make(map[int]*RemoteWriterUrl, len(Urls))
	for k, v := range Urls {
		writers[k] = NewRemoteWriterUrl(v)
	}
	return &RemoteCluster{
		Name:          name,
		uplen:         len(Urls),
		dimension:     dimension,
		filterLabels:  filterLabels,
		Writers:       writers,
		circuitBreaker: cb,
		stats: RemoteClusterStats{
			AvgLatency: 0,
		},
	}
}

func (r *RemoteCluster) Store(ctx context.Context, req []prompb.TimeSeries) error {
	if !r.circuitBreaker.Allow() {
		return ErrCircuitOpen
	}

	var wg sync.WaitGroup
	var mu sync.Mutex
	var firstError error

	sendSamplesChan := make(map[int][]prompb.TimeSeries, r.uplen)

	for _, ts := range req {
		if len(ts.Labels) == 0 {
			continue
		}
		if r.uplen > 1 {
			hash := common.SortLabelsHashKey(ts.Labels)
			dime := common.JumpConsistentHash(uint64(hash), r.dimension)
			ts.Labels = append(ts.Labels, prompb.Label{
				Name:  "stream_task_id",
				Value: fmt.Sprintf("%d", dime),
			})
			sendSeries := prompb.TimeSeries{
				Labels:  ts.Labels,
				Samples: ts.GetSamples(),
			}
			hashnode := hash
			if len(r.filterLabels) > 0 {
				var tmpLabels = []prompb.Label{}
				for _, rlabel := range r.filterLabels {
					for _, label := range ts.Labels {
						if label.Name == rlabel {
							tmpLabels = append(tmpLabels, label)
						}
					}
				}
				hashnode = common.SortLabelsHashKey(tmpLabels)
			}
			tmpch := common.JumpConsistentHash(uint64(hashnode), r.uplen)
			if _, ok := r.Writers[tmpch]; ok {
				mu.Lock()
				sendSamplesChan[tmpch] = append(sendSamplesChan[tmpch], sendSeries)
				mu.Unlock()
			}
		} else {
			sendSeries := prompb.TimeSeries{
				Labels:  ts.Labels,
				Samples: ts.GetSamples(),
			}
			mu.Lock()
			sendSamplesChan[0] = append(sendSamplesChan[0], sendSeries)
			mu.Unlock()
		}
	}

	r.mu.Lock()
	r.stats.TotalRequests++
	r.stats.LastRequestAt = time.Now()
	r.mu.Unlock()

	errChan := make(chan error, len(sendSamplesChan))

	for index, tsdata := range sendSamplesChan {
		wg.Add(1)
		go func(idx int, data []prompb.TimeSeries) {
			defer wg.Done()
			if _, err := r.Writers[idx].Store(ctx, data); err != nil {
				errChan <- err
			} else {
				errChan <- nil
			}
		}(index, tsdata)
	}

	go func() {
		wg.Wait()
		close(errChan)
	}()

	for err := range errChan {
		if err != nil {
			firstError = err
			r.circuitBreaker.RecordFailure()
			r.mu.Lock()
			r.stats.FailureCount++
			r.stats.LastFailureAt = time.Now()
			r.mu.Unlock()
		} else {
			r.circuitBreaker.RecordSuccess()
			r.mu.Lock()
			r.stats.SuccessCount++
			r.stats.LastSuccessAt = time.Now()
			r.mu.Unlock()
		}
	}

	if firstError != nil {
		return firstError
	}

	return nil
}

func (r *RemoteCluster) IsHealthy() bool {
	if !r.circuitBreaker.Allow() {
		return false
	}
	for _, writer := range r.Writers {
		if writer.Addr == "" {
			continue
		}
	}
	return true
}

func (r *RemoteCluster) GetStats() map[string]interface{} {
	r.mu.RLock()
	defer r.mu.RUnlock()

	cbStats := r.circuitBreaker.Stats()

	return map[string]interface{}{
		"name":               r.Name,
		"state":             cbStats.State,
		"total_requests":     r.stats.TotalRequests,
		"success_count":      r.stats.SuccessCount,
		"failure_count":      r.stats.FailureCount,
		"last_request_at":    r.stats.LastRequestAt.Format(time.RFC3339),
		"last_success_at":   r.stats.LastSuccessAt.Format(time.RFC3339),
		"last_failure_at":   r.stats.LastFailureAt.Format(time.RFC3339),
		"avg_latency_ms":    r.stats.AvgLatency,
		"circuit_breaker":   cbStats,
	}
}

