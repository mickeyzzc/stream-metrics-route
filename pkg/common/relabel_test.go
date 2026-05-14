package common

import (
	"reflect"
	"testing"

	"github.com/prometheus/prometheus/prompb"
)

// --- hashMod ---

func TestHashMod(t *testing.T) {
	tests := []struct {
		name string
		m    int
		key  uint32
		want int
	}{
		{"m=0 returns 0", 0, 100, 0},
		{"m=1 returns 0", 1, 100, 0},
		{"m=1 with key=0 returns 0", 1, 0, 0},
		{"m>1 basic mod", 10, 100, 0},       // 100 % 10 = 0
		{"m>1 with remainder", 10, 137, 7},  // 137 % 10 = 7
		{"m>1 key < m", 100, 42, 42},        // 42 % 100 = 42
		{"m>1 key=0", 10, 0, 0},
		{"m negative returns 0", -5, 100, 0},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := hashMod(tc.m, tc.key)
			if got != tc.want {
				t.Errorf("hashMod(%d, %d) = %d, want %d", tc.m, tc.key, got, tc.want)
			}
		})
	}
}

func TestHashModConsistency(t *testing.T) {
	m, key := 16, uint32(12345)
	first := hashMod(m, key)
	for i := 0; i < 100; i++ {
		got := hashMod(m, key)
		if got != first {
			t.Fatalf("hashMod not consistent: got %d on iteration %d, want %d", got, i, first)
		}
	}
}

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
	h1 := sortLabelsHashKey(set1)
	h2 := sortLabelsHashKey(set2)
	if h1 != h2 {
		t.Errorf("sortLabelsHashKey order dependent: %d != %d", h1, h2)
	}
}

func TestSortLabelsHashKeyConsistency(t *testing.T) {
	labels := []prompb.Label{
		{Name: "job", Value: "node"},
		{Name: "__name__", Value: "up"},
	}
	first := sortLabelsHashKey(labels)
	for i := 0; i < 100; i++ {
		got := sortLabelsHashKey(labels)
		if got != first {
			t.Fatalf("sortLabelsHashKey not consistent on iteration %d: got %d, want %d", i, got, first)
		}
	}
}

// --- sortLabelsHashMod ---

func TestSortLabelsHashModEmpty(t *testing.T) {
	got := sortLabelsHashMod(10, nil)
	if got != 0 {
		t.Errorf("sortLabelsHashMod(10, nil) = %d, want 0", got)
	}
	got = sortLabelsHashMod(10, []prompb.Label{})
	if got != 0 {
		t.Errorf("sortLabelsHashMod(10, []) = %d, want 0", got)
	}
}

func TestSortLabelsHashModConsistency(t *testing.T) {
	labels := []prompb.Label{
		{Name: "job", Value: "node"},
		{Name: "__name__", Value: "up"},
	}
	m := 16
	first := sortLabelsHashMod(m, labels)
	for i := 0; i < 100; i++ {
		got := sortLabelsHashMod(m, labels)
		if got != first {
			t.Fatalf("sortLabelsHashMod not consistent on iteration %d: got %d, want %d", i, got, first)
		}
	}
}

func TestSortLabelsHashModOrderIndependent(t *testing.T) {
	set1 := []prompb.Label{
		{Name: "__name__", Value: "up"},
		{Name: "job", Value: "node"},
	}
	set2 := []prompb.Label{
		{Name: "job", Value: "node"},
		{Name: "__name__", Value: "up"},
	}
	m := 8
	h1 := sortLabelsHashMod(m, set1)
	h2 := sortLabelsHashMod(m, set2)
	if h1 != h2 {
		t.Errorf("sortLabelsHashMod order dependent: %d != %d", h1, h2)
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
