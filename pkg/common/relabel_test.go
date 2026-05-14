package common

import (
	"reflect"
	"testing"

	"github.com/prometheus/prometheus/prompb"
)


// --- sortLabels ---

func TestSortLabelsEmpty(t *testing.T) {
	got := sortLabels(nil)
	if got != nil {
		t.Errorf("sortLabels(nil) = %v, want nil", got)
	}
	got = sortLabels([]prompb.Label{})
	if got != nil {
		t.Errorf("sortLabels([]) = %v, want nil", got)
	}
}

func TestSortLabelsSorting(t *testing.T) {
	labels := []prompb.Label{
		{Name: "job", Value: "node"},
		{Name: "__name__", Value: "up"},
		{Name: "instance", Value: "localhost:9090"},
	}
	got := sortLabels(labels)

	// Expected: interleaved names+values, sorted alphabetically
	// Names: __name__, instance, job
	// Values: localhost:9090, node, up
	// Combined sorted: __name__, instance, job, localhost:9090, node, up
	want := []string{"__name__", "instance", "job", "localhost:9090", "node", "up"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("sortLabels() = %v, want %v", got, want)
	}
}

// --- sortLabelsHashKey ---

func TestSortLabelsHashKeyOrderIndependent(t *testing.T) {
	set1 := []prompb.Label{
		{Name: "__name__", Value: "up"},
		{Name: "job", Value: "node"},
	}
	set2 := []prompb.Label{
		{Name: "job", Value: "node"},
		{Name: "__name__", Value: "up"},
	}
	h1 := SortLabelsHashKey(set1)
	h2 := SortLabelsHashKey(set2)
	if h1 != h2 {
		t.Errorf("sortLabelsHashKey order dependent: %d != %d", h1, h2)
	}
}

func TestSortLabelsHashKeyConsistency(t *testing.T) {
	labels := []prompb.Label{
		{Name: "job", Value: "node"},
		{Name: "__name__", Value: "up"},
	}
	first := SortLabelsHashKey(labels)
	for i := 0; i < 100; i++ {
		got := SortLabelsHashKey(labels)
		if got != first {
			t.Fatalf("sortLabelsHashKey not consistent on iteration %d: got %d, want %d", i, got, first)
		}
	}
}


// --- marshalLabelsFast / unmarshalLabelsFast ---

func TestMarshalUnmarshalRoundtrip(t *testing.T) {
	tests := []struct {
		name   string
		labels []prompb.Label
	}{
		{"single label", []prompb.Label{
			{Name: "__name__", Value: "up"},
		}},
		{"multiple labels", []prompb.Label{
			{Name: "__name__", Value: "up"},
			{Name: "job", Value: "node_exporter"},
			{Name: "instance", Value: "localhost:9100"},
		}},
		{"empty value", []prompb.Label{
			{Name: "env", Value: ""},
		}},
		{"empty name", []prompb.Label{
			{Name: "", Value: "val"},
		}},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			data := marshalLabelsFast(nil, tc.labels)
			got, err := unmarshalLabelsFast(nil, data)
			if err != nil {
				t.Fatalf("unmarshalLabelsFast returned error: %v", err)
			}
			if !reflect.DeepEqual(got, tc.labels) {
				t.Errorf("roundtrip failed:\n  got:  %v\n  want: %v", got, tc.labels)
			}
		})
	}
}

func TestMarshalUnmarshalRoundtripAppend(t *testing.T) {
	labels := []prompb.Label{
		{Name: "a", Value: "1"},
		{Name: "b", Value: "2"},
	}
	data := marshalLabelsFast(nil, labels)
	var dst []prompb.Label
	got, err := unmarshalLabelsFast(dst, data)
	if err != nil {
		t.Fatalf("unmarshalLabelsFast returned error: %v", err)
	}
	if !reflect.DeepEqual(got, labels) {
		t.Errorf("roundtrip with nil dst failed:\n  got:  %v\n  want: %v", got, labels)
	}
}

func TestUnmarshalLabelsFastInsufficientBytes(t *testing.T) {
	tests := []struct {
		name string
		data []byte
	}{
		{"nil", nil},
		{"1 byte", make([]byte, 1)},
		{"3 bytes", make([]byte, 3)},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := unmarshalLabelsFast(nil, tc.data)
			if err == nil {
				t.Error("expected error for insufficient bytes, got nil")
			}
		})
	}
}

func TestUnmarshalLabelsFastTruncatedBody(t *testing.T) {
	// Marshal valid labels, then truncate the result to trigger body parse errors
	labels := []prompb.Label{
		{Name: "job", Value: "node"},
		{Name: "instance", Value: "localhost:9100"},
	}
	full := marshalLabelsFast(nil, labels)

	// Cut after the count prefix (4 bytes) — should fail reading first label name length
	if _, err := unmarshalLabelsFast(nil, full[:4]); err == nil {
		t.Error("expected error for truncated body after count, got nil")
	}

	// Cut in the middle of the data
	if _, err := unmarshalLabelsFast(nil, full[:len(full)/2]); err == nil {
		t.Error("expected error for half-truncated body, got nil")
	}
}

func TestUnmarshalLabelsFastNonEmptyTail(t *testing.T) {
	labels := []prompb.Label{
		{Name: "a", Value: "b"},
	}
	data := marshalLabelsFast(nil, labels)
	// Append extra bytes to make a non-empty tail
	data = append(data, 0xFF, 0xFF, 0xFF, 0xFF)
	_, err := unmarshalLabelsFast(nil, data)
	if err == nil {
		t.Error("expected error for non-empty tail, got nil")
	}
}

// --- JumpConsistentHash ---

func TestJumpConsistentHash_EdgeCases(t *testing.T) {
	tests := []struct {
		name       string
		key        uint64
		numBuckets int
		want       int
	}{
		{"numBuckets=0 returns 0", 12345, 0, 0},
		{"numBuckets=-1 returns 0", 12345, -1, 0},
		{"numBuckets=1 returns 0", 12345, 1, 0},
		{"numBuckets=2 key=0", 0, 2, 0},
		{"numBuckets=2 key=1 in range", 1, 2, -1}, // -1 means check range only
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
		got := JumpConsistentHash(tc.key, tc.numBuckets)
		if tc.want == -1 {
			// Range-only check: verify result is in [0, numBuckets)
			if got < 0 || got >= tc.numBuckets {
				t.Errorf("JumpConsistentHash(%d, %d) = %d, out of range [0, %d)", tc.key, tc.numBuckets, got, tc.numBuckets)
			}
		} else if got != tc.want {
			t.Errorf("JumpConsistentHash(%d, %d) = %d, want %d", tc.key, tc.numBuckets, got, tc.want)
		}
		})
	}
}

func TestJumpConsistentHash_Consistency(t *testing.T) {
	key := uint64(9876543210)
	numBuckets := 16
	first := JumpConsistentHash(key, numBuckets)
	for i := 0; i < 100; i++ {
		got := JumpConsistentHash(key, numBuckets)
		if got != first {
			t.Fatalf("JumpConsistentHash not consistent on iteration %d: got %d, want %d", i, got, first)
		}
	}
}

func TestJumpConsistentHash_Distribution(t *testing.T) {
	const numKeys = 10000
	numBuckets := 10
	counts := make(map[int]int)
	for i := 0; i < numKeys; i++ {
		bucket := JumpConsistentHash(uint64(i), numBuckets)
		counts[bucket]++
	}
	for b := 0; b < numBuckets; b++ {
		c := counts[b]
		pct := float64(c) / float64(numKeys) * 100
		if pct < 5.0 || pct > 20.0 {
			t.Errorf("bucket %d: %.1f%% outside [5%%, 20%%] range (count=%d)", b, pct, c)
		}
	}
}

func TestJumpConsistentHash_Migration(t *testing.T) {
	const numKeys = 10000
	moved := 0
	for i := 0; i < numKeys; i++ {
		old := JumpConsistentHash(uint64(i), 3)
		new_ := JumpConsistentHash(uint64(i), 4)
		if old != new_ {
			moved++
		}
	}
	pct := float64(moved) / float64(numKeys) * 100
	if pct > 35.0 {
		t.Errorf("migration from 3\u21924 buckets: %.1f%% moved, expected \u2264 35%%", pct)
	}
	t.Logf("migration from 3\u21924: %d/%d keys moved (%.1f%%)", moved, numKeys, pct)
}
