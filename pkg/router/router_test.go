package router

import (
	"context"
	"errors"
	"testing"

	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/prompb"
)

// --- Inline mock for RemoteStore interface ---

type mockStore struct {
	storeFn   func(ctx context.Context, req []prompb.TimeSeries) error
	healthyFn func() bool
	statsFn   func() map[string]interface{}
}

func (m *mockStore) Store(ctx context.Context, req []prompb.TimeSeries) error {
	if m.storeFn != nil {
		return m.storeFn(ctx, req)
	}
	return nil
}

func (m *mockStore) IsHealthy() bool {
	if m.healthyFn != nil {
		return m.healthyFn()
	}
	return true
}

func (m *mockStore) GetStats() map[string]interface{} {
	if m.statsFn != nil {
		return m.statsFn()
	}
	return nil
}

// --- MultiError tests ---

func TestMultiError_Add_NilError(t *testing.T) {
	me := &MultiError{}
	me.Add(nil)
	if me.HasError() {
		t.Error("expected HasError() to be false after adding nil error")
	}
	if len(me.Errors) != 0 {
		t.Errorf("expected 0 errors, got %d", len(me.Errors))
	}
}

func TestMultiError_Add_SingleError(t *testing.T) {
	me := &MultiError{}
	err := errors.New("something failed")
	me.Add(err)
	if !me.HasError() {
		t.Error("expected HasError() to be true")
	}
	if len(me.Errors) != 1 {
		t.Errorf("expected 1 error, got %d", len(me.Errors))
	}
	if me.Errors[0].Error() != "something failed" {
		t.Errorf("unexpected error text: %s", me.Errors[0].Error())
	}
}

func TestMultiError_Add_MultipleErrors(t *testing.T) {
	me := &MultiError{}
	me.Add(errors.New("err1"))
	me.Add(nil) // should be ignored
	me.Add(errors.New("err2"))
	me.Add(errors.New("err3"))
	if !me.HasError() {
		t.Error("expected HasError() to be true")
	}
	if len(me.Errors) != 3 {
		t.Errorf("expected 3 errors, got %d", len(me.Errors))
	}
}

func TestMultiError_Error_Empty(t *testing.T) {
	me := &MultiError{}
	if got := me.Error(); got != "" {
		t.Errorf("expected empty string, got %q", got)
	}
}

func TestMultiError_Error_Single(t *testing.T) {
	me := &MultiError{}
	me.Add(errors.New("only one"))
	want := "only one"
	if got := me.Error(); got != want {
		t.Errorf("expected %q, got %q", want, got)
	}
}

func TestMultiError_Error_Multiple(t *testing.T) {
	me := &MultiError{}
	me.Add(errors.New("first"))
	me.Add(errors.New("second"))
	got := me.Error()
	if got == "" {
		t.Error("expected non-empty error string")
	}
	want := "2 errors: first error: first"
	if got != want {
		t.Errorf("expected %q, got %q", want, got)
	}
}

func TestMultiError_HasError_Empty(t *testing.T) {
	me := &MultiError{}
	if me.HasError() {
		t.Error("empty MultiError should not have errors")
	}
}

func TestMultiError_HasError_WithErrors(t *testing.T) {
	me := &MultiError{}
	me.Add(errors.New("x"))
	if !me.HasError() {
		t.Error("MultiError with added error should HasError()")
	}
}

// --- classifyError tests ---

func TestClassifyError_Timeout(t *testing.T) {
	got := classifyError(errors.New("request timeout exceeded"))
	if got != "timeout" {
		t.Errorf("expected 'timeout', got %q", got)
	}
}

func TestClassifyError_Connection(t *testing.T) {
	got := classifyError(errors.New("connection refused"))
	if got != "connection" {
		t.Errorf("expected 'connection', got %q", got)
	}
}

func TestClassifyError_CircuitBreaker(t *testing.T) {
	got := classifyError(errors.New("circuit breaker is open"))
	if got != "circuit_breaker" {
		t.Errorf("expected 'circuit_breaker', got %q", got)
	}
}

func TestClassifyError_Unknown(t *testing.T) {
	got := classifyError(errors.New("something unexpected"))
	if got != "unknown" {
		t.Errorf("expected 'unknown', got %q", got)
	}
}

func TestClassifyError_Empty(t *testing.T) {
	got := classifyError(errors.New(""))
	if got != "unknown" {
		t.Errorf("expected 'unknown' for empty error, got %q", got)
	}
}

// --- formatLabelSet tests ---

func TestFormatLabelSet(t *testing.T) {
	input := []prompb.Label{
		{Name: "__name__", Value: "up"},
		{Name: "job", Value: "node"},
	}
	got := formatLabelSet(input)
	want := labels.FromMap(map[string]string{
		"__name__": "up",
		"job":      "node",
	})
	if !labels.Equal(got, want) {
		t.Errorf("formatLabelSet mismatch:\n  got:  %v\n  want: %v", got, want)
	}
}

func TestFormatLabelSet_Empty(t *testing.T) {
	got := formatLabelSet(nil)
	if got.Len() != 0 {
		t.Errorf("expected empty labels, got %v", got)
	}
}

func TestFormatLabelSet_DuplicateKeys(t *testing.T) {
	input := []prompb.Label{
		{Name: "job", Value: "first"},
		{Name: "job", Value: "second"},
	}
	got := formatLabelSet(input)
	// map overwrite: last value wins
	want := labels.FromMap(map[string]string{"job": "second"})
	if !labels.Equal(got, want) {
		t.Errorf("formatLabelSet duplicate key mismatch:\n  got:  %v\n  want: %v", got, want)
	}
}

// --- Routers.Store tests ---

func TestRouters_Store_NoRouters(t *testing.T) {
	rs := &Routers{Routers: make(map[string]*Router)}
	results := rs.Store(context.Background(), []prompb.TimeSeries{})
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].Error == nil {
		t.Error("expected error for no routers configured")
	}
	if results[0].Error.Error() != "no routers configured" {
		t.Errorf("unexpected error: %v", results[0].Error)
	}
}

func TestRouters_Store_Success(t *testing.T) {
	storeCalled := false
	ms := &mockStore{
		storeFn: func(ctx context.Context, req []prompb.TimeSeries) error {
			storeCalled = true
			return nil
		},
	}
	rs := &Routers{
		Routers: map[string]*Router{
			"test-router": {
				Name:        "test-router",
				RemoteStore: ms,
			},
		},
	}

	ts := []prompb.TimeSeries{
		{Labels: []prompb.Label{{Name: "__name__", Value: "test_metric"}}},
	}
	results := rs.Store(context.Background(), ts)

	if !storeCalled {
		t.Error("expected Store to be called on mock")
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].Error != nil {
		t.Errorf("expected no error, got %v", results[0].Error)
	}
	if results[0].RouterName != "test-router" {
		t.Errorf("expected router name 'test-router', got %q", results[0].RouterName)
	}
	if results[0].Count != 1 {
		t.Errorf("expected count 1, got %d", results[0].Count)
	}
}

func TestRouters_Store_Error(t *testing.T) {
	ms := &mockStore{
		storeFn: func(ctx context.Context, req []prompb.TimeSeries) error {
			return errors.New("connection refused to backend")
		},
	}
	rs := &Routers{
		Routers: map[string]*Router{
			"err-router": {
				Name:        "err-router",
				RemoteStore: ms,
			},
		},
	}

	ts := []prompb.TimeSeries{
		{Labels: []prompb.Label{{Name: "__name__", Value: "test_metric"}}},
	}
	results := rs.Store(context.Background(), ts)

	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].Error == nil {
		t.Error("expected error result")
	}
	if results[0].RouterName != "err-router" {
		t.Errorf("expected 'err-router', got %q", results[0].RouterName)
	}
}

// --- Routers.IsHealthy tests ---

func TestRouters_IsHealthy_AllHealthy(t *testing.T) {
	rs := &Routers{
		Routers: map[string]*Router{
			"a": {Name: "a", RemoteStore: &mockStore{healthyFn: func() bool { return true }}},
			"b": {Name: "b", RemoteStore: &mockStore{healthyFn: func() bool { return true }}},
		},
	}
	if !rs.IsHealthy() {
		t.Error("expected healthy when all backends healthy")
	}
}

func TestRouters_IsHealthy_OneUnhealthy(t *testing.T) {
	rs := &Routers{
		Routers: map[string]*Router{
			"a": {Name: "a", RemoteStore: &mockStore{healthyFn: func() bool { return true }}},
			"b": {Name: "b", RemoteStore: &mockStore{healthyFn: func() bool { return false }}},
		},
	}
	if rs.IsHealthy() {
		t.Error("expected unhealthy when one backend is unhealthy")
	}
}

func TestRouters_IsHealthy_Empty(t *testing.T) {
	rs := &Routers{Routers: make(map[string]*Router)}
	if !rs.IsHealthy() {
		t.Error("empty routers should be healthy (no unhealthy backends)")
	}
}

// --- Routers.GetRouterStats tests ---

func TestRouters_GetRouterStats(t *testing.T) {
	rs := &Routers{
		Routers: map[string]*Router{
			"r1": {
				Name: "r1",
				RemoteStore: &mockStore{statsFn: func() map[string]interface{} {
					return map[string]interface{}{"shards": 3}
				}},
			},
			"r2": {
				Name: "r2",
				RemoteStore: &mockStore{statsFn: func() map[string]interface{} {
					return map[string]interface{}{"shards": 5}
				}},
			},
		},
	}

	stats := rs.GetRouterStats()
	if len(stats) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(stats))
	}
	r1, ok := stats["r1"]
	if !ok {
		t.Error("missing stats for r1")
	}
	r1Map := r1.(map[string]interface{})
	if r1Map["shards"] != 3 {
		t.Errorf("expected r1 shards=3, got %v", r1Map["shards"])
	}
	r2, ok := stats["r2"]
	if !ok {
		t.Error("missing stats for r2")
	}
	r2Map := r2.(map[string]interface{})
	if r2Map["shards"] != 5 {
		t.Errorf("expected r2 shards=5, got %v", r2Map["shards"])
	}
}

func TestRouters_GetRouterStats_Empty(t *testing.T) {
	rs := &Routers{Routers: make(map[string]*Router)}
	stats := rs.GetRouterStats()
	if stats == nil {
		t.Error("expected non-nil map")
	}
	if len(stats) != 0 {
		t.Errorf("expected empty map, got %d entries", len(stats))
	}
}
