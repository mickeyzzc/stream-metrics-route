package remote

import (
	"github.com/gogo/protobuf/proto"
	"github.com/golang/snappy"
	"github.com/prometheus/prometheus/prompb"
)

func buildWriteRequest(samples []prompb.TimeSeries, metadata []prompb.MetricMetadata, pBuf *proto.Buffer, buf []byte) ([]byte, error) {

	req := &prompb.WriteRequest{
		Timeseries: samples,
		Metadata:   metadata,
	}

	if pBuf == nil {
		pBuf = proto.NewBuffer(nil)
	} else {
		pBuf.Reset()
	}
	err := pBuf.Marshal(req)
	if err != nil {
		return nil, err
	}

	if buf != nil {
		buf = buf[0:cap(buf)]
	}
	compressed := snappy.Encode(buf, pBuf.Bytes())
	return compressed, nil
}

func hashMod(m int, key uint32) int {
	if m <= 1 {
		return 0
	}
	return int(key % uint32(m))
}
