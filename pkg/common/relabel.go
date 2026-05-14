package common

import (
	"fmt"
	"hash/fnv"
	"sort"

	"github.com/VictoriaMetrics/VictoriaMetrics/lib/bytesutil"
	"github.com/VictoriaMetrics/VictoriaMetrics/lib/encoding"
	"github.com/prometheus/prometheus/prompb"
)

var bbPool bytesutil.ByteBufferPool

func marshalLabelsFast(dst []byte, labels []prompb.Label) []byte {
	dst = encoding.MarshalUint32(dst, uint32(len(labels)))
	for _, label := range labels {
		dst = encoding.MarshalUint32(dst, uint32(len(label.Name)))
		dst = append(dst, label.Name...)
		dst = encoding.MarshalUint32(dst, uint32(len(label.Value)))
		dst = append(dst, label.Value...)
	}
	return dst
}

func unmarshalLabelsFast(dst []prompb.Label, src []byte) ([]prompb.Label, error) {
	if len(src) < 4 {
		return dst, fmt.Errorf("cannot unmarshal labels count from %d bytes; needs at least 4 bytes", len(src))
	}
	n := encoding.UnmarshalUint32(src)
	src = src[4:]
	for i := uint32(0); i < n; i++ {
		// Unmarshal label name
		if len(src) < 4 {
			return dst, fmt.Errorf("cannot unmarshal label name length from %d bytes; needs at least 4 bytes", len(src))
		}
		labelNameLen := encoding.UnmarshalUint32(src)
		src = src[4:]
		if uint32(len(src)) < labelNameLen {
			return dst, fmt.Errorf("cannot unmarshal label name from %d bytes; needs at least %d bytes", len(src), labelNameLen)
		}
		labelName := bytesutil.InternBytes(src[:labelNameLen])
		src = src[labelNameLen:]

		// Unmarshal label value
		if len(src) < 4 {
			return dst, fmt.Errorf("cannot unmarshal label value length from %d bytes; needs at least 4 bytes", len(src))
		}
		labelValueLen := encoding.UnmarshalUint32(src)
		src = src[4:]
		if uint32(len(src)) < labelValueLen {
			return dst, fmt.Errorf("cannot unmarshal label value from %d bytes; needs at least %d bytes", len(src), labelValueLen)
		}
		labelValue := bytesutil.InternBytes(src[:labelValueLen])
		src = src[labelValueLen:]

		dst = append(dst, prompb.Label{
			Name:  labelName,
			Value: labelValue,
		})
	}
	if len(src) > 0 {
		return dst, fmt.Errorf("unexpected non-empty tail after unmarshaling labels; tail length is %d bytes", len(src))
	}
	return dst, nil
}

func sortLabels(labels []prompb.Label) []string {
	if len(labels) == 0 {
		return nil
	}
	newLabel := make([]string, 0, len(labels))
	for _, lal := range labels {
		newLabel = append(newLabel, lal.Name)
		newLabel = append(newLabel, lal.Value)
	}

	sort.Strings(newLabel)

	return newLabel
}

// JumpConsistentHash implements the jump consistent hash algorithm from
// "A Fast, Minimal Memory, Consistent Hash Algorithm" by Lamping & Veach (2014).
// It maps a key to a bucket in [0, numBuckets) with O(1) time and zero memory overhead.
// When numBuckets changes from N to N+1, only ~1/(N+1) of keys are remapped.
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

func SortLabelsHashKey(labels []prompb.Label) uint32 {
	newLabel := make([]string, 0, len(labels)*2)
	for _, lal := range labels {
		newLabel = append(newLabel, lal.Name)
		newLabel = append(newLabel, lal.Value)
	}

	sort.Strings(newLabel)
	h := fnv.New32a()
	for _, v := range newLabel {
		h.Write([]byte(v))
	}
	return h.Sum32()
}

func absInt(num int) int {
	if num < 0 {
		return -num
	}
	return num
}
