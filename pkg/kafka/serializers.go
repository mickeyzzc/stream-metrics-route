package kafka

import (
	"bytes"
	"encoding/json"
	"io/ioutil"
	"strconv"
	"text/template"
	"time"

	"github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/prompb"

	"github.com/linkedin/goavro"
)

// Serializer represents an abstract metrics serializer
type Serializer interface {
	Marshal(metric map[string]interface{}) ([]byte, error)
}

// Serialize generates the JSON representation for a given Prometheus metric.
func Serialize(id string, topicTemplate template.Template, s Serializer, req []prompb.TimeSeries) (map[string][][]byte, error) {
	promBatches.WithLabelValues(id).Add(float64(1))
	result := make(map[string][][]byte)

	for _, ts := range req {
		labels := make(map[string]string, len(ts.Labels))

		for _, l := range ts.Labels {
			labels[string(model.LabelName(l.Name))] = string(model.LabelValue(l.Value))
		}

		t := topic(topicTemplate, labels)

		for _, sample := range ts.Samples {
			name := string(labels["__name__"])
			defaultTelemetry.Logger.Debug("kafka filter samples", "name", name, "labels", labels)
			if !filter(name, labels) {
				objectsFiltered.WithLabelValues(id).Add(float64(1))
				continue
			}

			epoch := time.Unix(sample.Timestamp/1000, 0).UTC()
			m := map[string]interface{}{
				"timestamp": epoch.Format(time.RFC3339),
				"value":     strconv.FormatFloat(sample.Value, 'f', -1, 64),
				"name":      name,
				"labels":    labels,
			}

			data, err := s.Marshal(m)
			if err != nil {
				serializeFailed.WithLabelValues(id).Add(float64(1))
				defaultTelemetry.Logger.Error("couldn't marshal timeseries ", err)
			}
			serializeTotal.WithLabelValues(id).Add(float64(1))
			result[t] = append(result[t], data)
		}
	}

	return result, nil
}

// JSONSerializer represents a metrics serializer that writes JSON
type JSONSerializer struct {
}

func (s *JSONSerializer) Marshal(metric map[string]interface{}) ([]byte, error) {
	return json.Marshal(metric)
}

func NewJSONSerializer() (*JSONSerializer, error) {
	return &JSONSerializer{}, nil
}

// AvroJSONSerializer represents a metrics serializer that writes Avro-JSON
type AvroJSONSerializer struct {
	codec *goavro.Codec
}

func (s *AvroJSONSerializer) Marshal(metric map[string]interface{}) ([]byte, error) {
	return s.codec.TextualFromNative(nil, metric)
}

// NewAvroJSONSerializer builds a new instance of the AvroJSONSerializer
func NewAvroJSONSerializer(schemaPath string) (*AvroJSONSerializer, error) {
	schema, err := ioutil.ReadFile(schemaPath)
	if err != nil {
		defaultTelemetry.Logger.Error("couldn't read avro schema", err)
		return nil, err
	}

	codec, err := goavro.NewCodec(string(schema))
	if err != nil {
		defaultTelemetry.Logger.Error("couldn't create avro codec", err)
		return nil, err
	}

	return &AvroJSONSerializer{
		codec: codec,
	}, nil
}

func processWriteRequest(id string, topicTemplate template.Template, req []prompb.TimeSeries) (map[string][][]byte, error) {
	//defaultTelemetry.Logger.Debug("processing write request", "var :", req)
	return Serialize(id, topicTemplate, serializer, req)
}

func topic(topicTemplate template.Template, labels map[string]string) string {
	var buf bytes.Buffer
	if err := topicTemplate.Execute(&buf, labels); err != nil {
		return ""
	}
	return buf.String()
}

func filter(name string, labels map[string]string) bool {
	if len(match) == 0 {
		return true
	}
	mf, ok := match[name]
	if !ok {
		return false
	}

	for _, m := range mf.Metric {
		if len(m.Label) == 0 {
			return true
		}

		labelMatch := true
		for _, label := range m.Label {
			val, ok := labels[label.GetName()]
			if !ok || val != label.GetValue() {
				labelMatch = false
				break
			}
		}

		if labelMatch {
			return true
		}
	}
	return false
}
