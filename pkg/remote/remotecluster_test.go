package remote

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"stream-metrics-route/pkg/common"

	"github.com/gogo/protobuf/proto"
	"github.com/golang/snappy"
	"github.com/prometheus/prometheus/prompb"
)

// --- Test helpers ---

// captureTransport is a mock http.RoundTripper that captures request bodies
// and responds with 204 No Content.
type captureTransport struct {
	mu       sync.Mutex
	captured []prompb.WriteRequest
}

func (t *captureTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	body, err := io.ReadAll(req.Body)
	if err != nil {
		return nil, err
	}
	decompressed, err := snappy.Decode(nil, body)
	if err != nil {
		return nil, err
	}
	var wr prompb.WriteRequest
	if err := proto.Unmarshal(decompressed, &wr); err != nil {
		return nil, err
	}
	t.mu.Lock()
	t.captured = append(t.captured, wr)
	t.mu.Unlock()
	return &http.Response{
		StatusCode: http.StatusNoContent,
		Body:       io.NopCloser(strings.NewReader("")),
		Header:     make(http.Header),
	}, nil
}

func (t *captureTransport) totalTimeseries() int {
	t.mu.Lock()
	defer t.mu.Unlock()
	total := 0
	for _, wr := range t.captured {
		total += len(wr.Timeseries)
	}
	return total
}

func (t *captureTransport) allTimeseries() []prompb.TimeSeries {
	t.mu.Lock()
	defer t.mu.Unlock()
	var result []prompb.TimeSeries
	for _, wr := range t.captured {
		result = append(result, wr.Timeseries...)
	}
	return result
}

// newTestCluster creates a RemoteCluster with numBackends mock backends.
// Each backend's HTTP client is replaced with a captureTransport so no real
// network calls are made.
func newTestCluster(name string, dimension int, filterLabels []string, numBackends int) (*RemoteCluster, []*captureTransport) {
	urls := make([]string, numBackends)
	for i := 0; i < numBackends; i++ {
		urls[i] = fmt.Sprintf("http://backend-%d:8429/api/v1/write", i)
	}
	cluster := NewRemoteCluster(name, dimension, filterLabels, urls)

	transports := make([]*captureTransport, numBackends)
	for i := 0; i < numBackends; i++ {
		transport := &captureTransport{}
		transports[i] = transport
		cluster.Writers[i].httpClient = &http.Client{Transport: transport}
	}
	return cluster, transports
}

// makeLabels creates a sorted prompb.Label slice from a map.
func makeLabels(kv map[string]string) []prompb.Label {
	labels := make([]prompb.Label, 0, len(kv))
	for k, v := range kv {
		labels = append(labels, prompb.Label{Name: k, Value: v})
	}
	sort.Slice(labels, func(i, j int) bool {
		return labels[i].Name < labels[j].Name
	})
	return labels
}

// copyLabels returns a copy of the label slice.
func copyLabels(src []prompb.Label) []prompb.Label {
	dst := make([]prompb.Label, len(src))
	copy(dst, src)
	return dst
}

// --- Tests ---

func TestStore_Determinism(t *testing.T) {
	cluster, transports := newTestCluster("test-determinism", 100, nil, 3)

	labels := makeLabels(map[string]string{
		"__name__": "up",
		"job":      "node",
	})

	// Compute expected backend index using the same algorithm as Store()
	hash := common.SortLabelsHashKey(labels)
	expectedIdx := common.JumpConsistentHash(uint64(hash), cluster.uplen)

	// Call Store 100 times with identical label sets
	for i := 0; i < 100; i++ {
		ts := prompb.TimeSeries{
			Labels:  copyLabels(labels),
			Samples: []prompb.Sample{{Value: 1.0, Timestamp: int64(i)}},
		}
		err := cluster.Store(context.Background(), []prompb.TimeSeries{ts})
		if err != nil {
			t.Fatalf("call %d: Store failed: %v", i, err)
		}
	}

	// All 100 time series must have gone to the same backend
	for idx, tr := range transports {
		got := tr.totalTimeseries()
		if idx == expectedIdx {
			if got != 100 {
				t.Errorf("backend %d (expected): got %d time series, want 100", idx, got)
			}
		} else {
			if got != 0 {
				t.Errorf("backend %d: got %d time series, want 0", idx, got)
			}
		}
	}
}

func TestStore_Distribution(t *testing.T) {
	numBackends := 3
	cluster, transports := newTestCluster("test-distribution", 100, nil, numBackends)

	// Generate 10000 unique time series with varying labels
	const totalTS = 10000
	allTS := make([]prompb.TimeSeries, totalTS)
	for i := 0; i < totalTS; i++ {
		allTS[i] = prompb.TimeSeries{
			Labels: makeLabels(map[string]string{
				"__name__": fmt.Sprintf("metric_%d", i),
				"job":      fmt.Sprintf("job_%d", i%100),
				"instance": fmt.Sprintf("host-%d", i),
			}),
			Samples: []prompb.Sample{{Value: float64(i), Timestamp: int64(i)}},
		}
	}

	err := cluster.Store(context.Background(), allTS)
	if err != nil {
		t.Fatalf("Store failed: %v", err)
	}

	total := 0
	for idx, tr := range transports {
		c := tr.totalTimeseries()
		total += c
		t.Logf("backend %d: %d time series", idx, c)
	}

	if total != totalTS {
		t.Errorf("total captured = %d, want %d", total, totalTS)
	}

	// With 3 backends expect ~33% each. Allow [25%, 45%] for hash variance.
	for idx, tr := range transports {
		c := tr.totalTimeseries()
		pct := float64(c) / float64(totalTS) * 100
		if pct < 25.0 || pct > 45.0 {
			t.Errorf("backend %d: %.1f%% outside [25%%, 45%%] range (count=%d)", idx, pct, c)
		}
	}
}

func TestStore_FilterLabels(t *testing.T) {
	filterLabels := []string{"__name__", "job"}
	cluster, transports := newTestCluster("test-filter", 100, filterLabels, 3)

	// Create 5 time series that differ ONLY in the "instance" label.
	// Because filterLabels=["__name__","job"], all should route to the same backend.
	const numTS = 5
	allTS := make([]prompb.TimeSeries, numTS)
	for i := 0; i < numTS; i++ {
		allTS[i] = prompb.TimeSeries{
			Labels: makeLabels(map[string]string{
				"__name__": "up",
				"job":      "node",
				"instance": fmt.Sprintf("host-%d", i),
			}),
			Samples: []prompb.Sample{{Value: 1.0, Timestamp: int64(i)}},
		}
	}

	err := cluster.Store(context.Background(), allTS)
	if err != nil {
		t.Fatalf("Store failed: %v", err)
	}

	nonEmptyBackends := 0
	totalRouted := 0
	for _, tr := range transports {
		c := tr.totalTimeseries()
		if c > 0 {
			nonEmptyBackends++
			totalRouted += c
		}
	}

	if nonEmptyBackends != 1 {
		t.Errorf("expected exactly 1 backend to receive data, got %d", nonEmptyBackends)
	}
	if totalRouted != numTS {
		t.Errorf("total routed = %d, want %d", totalRouted, numTS)
	}
}

func TestStore_EdgeCases(t *testing.T) {
	t.Run("EmptySlice", func(t *testing.T) {
		cluster, _ := newTestCluster("test-empty", 100, nil, 3)
		err := cluster.Store(context.Background(), []prompb.TimeSeries{})
		if err != nil {
			t.Errorf("expected no error for empty slice, got %v", err)
		}
	})

	t.Run("EmptyLabels_Skipped", func(t *testing.T) {
		cluster, transports := newTestCluster("test-empty-labels", 100, nil, 3)
		ts := prompb.TimeSeries{
			Labels:  []prompb.Label{},
			Samples: []prompb.Sample{{Value: 1.0, Timestamp: 1000}},
		}
		err := cluster.Store(context.Background(), []prompb.TimeSeries{ts})
		if err != nil {
			t.Errorf("expected no error for empty labels, got %v", err)
		}
		for idx, tr := range transports {
			if c := tr.totalTimeseries(); c != 0 {
				t.Errorf("backend %d: got %d time series, want 0", idx, c)
			}
		}
	})

	t.Run("NilLabels_Skipped", func(t *testing.T) {
		cluster, transports := newTestCluster("test-nil-labels", 100, nil, 3)
		ts := prompb.TimeSeries{
			Labels:  nil,
			Samples: []prompb.Sample{{Value: 1.0, Timestamp: 1000}},
		}
		err := cluster.Store(context.Background(), []prompb.TimeSeries{ts})
		if err != nil {
			t.Errorf("expected no error for nil labels, got %v", err)
		}
		for idx, tr := range transports {
			if c := tr.totalTimeseries(); c != 0 {
				t.Errorf("backend %d: got %d time series, want 0", idx, c)
			}
		}
	})

	t.Run("SingleBackend", func(t *testing.T) {
		cluster, transports := newTestCluster("test-single", 100, nil, 1)
		ts := prompb.TimeSeries{
			Labels:  makeLabels(map[string]string{"__name__": "test"}),
			Samples: []prompb.Sample{{Value: 1.0, Timestamp: 1000}},
		}
		err := cluster.Store(context.Background(), []prompb.TimeSeries{ts})
		if err != nil {
			t.Fatalf("Store failed: %v", err)
		}
		if c := transports[0].totalTimeseries(); c != 1 {
			t.Errorf("backend 0: got %d time series, want 1", c)
		}
	})
}

func TestStore_TaskIDInjection(t *testing.T) {
	dimension := 100
	cluster, transports := newTestCluster("test-taskid", dimension, nil, 3)

	labels := makeLabels(map[string]string{
		"__name__": "up",
		"job":      "node",
	})
	ts := prompb.TimeSeries{
		Labels:  copyLabels(labels),
		Samples: []prompb.Sample{{Value: 1.0, Timestamp: 1000}},
	}

	err := cluster.Store(context.Background(), []prompb.TimeSeries{ts})
	if err != nil {
		t.Fatalf("Store failed: %v", err)
	}

	// Find the backend that received data
	var captured []prompb.TimeSeries
	for _, tr := range transports {
		if ts := tr.allTimeseries(); len(ts) > 0 {
			captured = ts
			break
		}
	}

	if len(captured) != 1 {
		t.Fatalf("captured time series count = %d, want 1", len(captured))
	}

	// Find stream_task_id label in the captured time series
	var taskIDStr string
	for _, l := range captured[0].Labels {
		if l.Name == "stream_task_id" {
			taskIDStr = l.Value
			break
		}
	}

	if taskIDStr == "" {
		t.Fatal("stream_task_id label not found in routed time series")
	}

	taskID, err := strconv.Atoi(taskIDStr)
	if err != nil {
		t.Fatalf("stream_task_id %q is not a valid integer", taskIDStr)
	}

	if taskID < 0 || taskID >= dimension {
		t.Errorf("stream_task_id = %d, out of range [0, %d)", taskID, dimension)
	}

	// Verify it matches the expected computation
	expectedDime := common.JumpConsistentHash(uint64(common.SortLabelsHashKey(labels)), dimension)
	if taskID != expectedDime {
		t.Errorf("stream_task_id = %d, want %d", taskID, expectedDime)
	}
}

func TestStore_Concurrent(t *testing.T) {
	numBackends := 3
	cluster, transports := newTestCluster("test-concurrent", 100, nil, numBackends)

	const numGoroutines = 10
	const tsPerGoroutine = 100

	var wg sync.WaitGroup
	errCh := make(chan error, numGoroutines)
	var totalSent atomic.Int64

	for g := 0; g < numGoroutines; g++ {
		wg.Add(1)
		go func(goroutineID int) {
			defer wg.Done()
			batch := make([]prompb.TimeSeries, tsPerGoroutine)
			for i := 0; i < tsPerGoroutine; i++ {
				batch[i] = prompb.TimeSeries{
					Labels: makeLabels(map[string]string{
						"__name__": fmt.Sprintf("metric_g%d_i%d", goroutineID, i),
						"job":      "test",
					}),
					Samples: []prompb.Sample{{Value: float64(i), Timestamp: int64(i)}},
				}
			}
			if err := cluster.Store(context.Background(), batch); err != nil {
				errCh <- fmt.Errorf("goroutine %d: %w", goroutineID, err)
				return
			}
			totalSent.Add(int64(tsPerGoroutine))
		}(g)
	}

	wg.Wait()
	close(errCh)

	for err := range errCh {
		t.Error(err)
	}

	totalCaptured := 0
	for _, tr := range transports {
		totalCaptured += tr.totalTimeseries()
	}

	expected := numGoroutines * tsPerGoroutine
	if sent := totalSent.Load(); sent != int64(expected) {
		t.Errorf("total sent = %d, want %d", sent, expected)
	}
	if totalCaptured != expected {
		t.Errorf("total captured = %d, want %d", totalCaptured, expected)
	}
}
