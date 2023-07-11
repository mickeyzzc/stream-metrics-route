package kafka

import (
	"context"
	"fmt"
	"net/http"
	"stream-metrics-route/pkg/setting"
	"strings"
	"text/template"

	kafkaclient "github.com/confluentinc/confluent-kafka-go/kafka"
	dto "github.com/prometheus/client_model/go"
	"github.com/prometheus/common/expfmt"
	"github.com/prometheus/prometheus/prompb"
	"gopkg.in/yaml.v2"
)

var (
	match = make(map[string]*dto.MetricFamily, 0)

	kafkaCompression      = "none"
	kafkaBatchNumMessages = "10000"

	serializer Serializer
)

type KafkaClient struct {
	name          string
	Producer      kafkaclient.Producer
	TopicTemplate template.Template
}

func NewKafka(name string, cfg setting.KafkaConfig) (*KafkaClient, error) {
	//var err error
	topicTemplate, err := parseTopicTemplate(cfg.KafkaTopic)
	if err != nil {
		return nil, fmt.Errorf("couldn't parse the topic template", err)
	}
	matchList, err := parseMatchList(cfg.Match)
	if err != nil {
		return nil, fmt.Errorf("couldn't parse the match rules", err)
	}
	match = matchList
	if cfg.KafkaBatchNumMessages == "" {
		cfg.KafkaBatchNumMessages = kafkaBatchNumMessages
	}
	if cfg.KafkaCompression == "" {
		cfg.KafkaCompression = kafkaCompression
	}
	kafkaConfig := kafkaclient.ConfigMap{
		"bootstrap.servers":   cfg.KafkaBrokerList,
		"compression.codec":   cfg.KafkaCompression,
		"batch.num.messages":  cfg.KafkaBatchNumMessages,
		"go.batch.producer":   true,  // Enable batch producer (for increased performance).
		"go.delivery.reports": false, // per-message delivery reports to the Events() channel
	}

	if cfg.SecurityProtocol != "" {

		switch cfg.SecurityProtocol {
		case "ssl":
			ssl := cfg.KafkaSslClient
			if ssl.SslCACertFile == "" || ssl.SslClientCertFile == "" || ssl.SslClientKeyFile == "" {
				return nil, fmt.Errorf("invalid config: kafka security protocol is not ssl based but ssl config is provided")
			}
			kafkaConfig["ssl.ca.location"] = ssl.SslCACertFile              // CA certificate file for verifying the broker's certificate.
			kafkaConfig["ssl.certificate.location"] = ssl.SslClientCertFile // Client's certificate
			kafkaConfig["ssl.key.location"] = ssl.SslClientKeyFile          // Client's key
			kafkaConfig["ssl.key.password"] = ssl.SslClientKeyPass          // Key password, if any.
		case "sasl_ssl", "sasl_plaintext":
			sasl := cfg.KafkaSasl
			if sasl.SaslMechanism == "" || sasl.SaslPassword == "" || sasl.SaslUsername == "" {
				return nil, fmt.Errorf("invalid config: kafka security protocol is not sasl based but sasl config is provided")
			}
			kafkaConfig["sasl.mechanism"] = sasl.SaslMechanism
			kafkaConfig["sasl.username"] = sasl.SaslUsername
			kafkaConfig["sasl.password"] = sasl.SaslPassword
		default:
			return nil, fmt.Errorf("invalid config: kafka security protocol is not ssl based but ssl config is provided")
		}
		kafkaConfig["security.protocol"] = cfg.SecurityProtocol
	}

	producer, err := kafkaclient.NewProducer(&kafkaConfig)
	if err != nil {
		return nil, err
	}

	return &KafkaClient{
		name:          name,
		Producer:      *producer,
		TopicTemplate: *topicTemplate,
	}, nil
}

func (k *KafkaClient) Store(ctx context.Context, req []prompb.TimeSeries) (int, error) {
	defer ctx.Done()
	metricsPerTopic, err := processWriteRequest(k.name, k.TopicTemplate, req)
	if err != nil {
		return http.StatusInternalServerError, fmt.Errorf("couldn't process write request", err)
	}

	for topic, metrics := range metricsPerTopic {
		t := topic
		defaultTelemetry.Logger.Debug("write request", "name", k.name, "metricsPerTopic", t)
		part := kafkaclient.TopicPartition{
			Partition: kafkaclient.PartitionAny,
			Topic:     &t,
		}
		for _, metric := range metrics {
			objectsWritten.WithLabelValues(k.name).Add(float64(1))
			err := k.Producer.Produce(&kafkaclient.Message{
				TopicPartition: part,
				Value:          metric,
			}, nil)

			if err != nil {
				objectsFailed.WithLabelValues(k.name).Add(float64(1))
				defaultTelemetry.Logger.Error("Failing metric", "err", err, "metric", string(metric))

				return http.StatusInternalServerError, err
			}
		}
	}
	return 0, nil
}

func parseMatchList(text string) (map[string]*dto.MetricFamily, error) {
	var matchRules []string
	err := yaml.Unmarshal([]byte(text), &matchRules)
	if err != nil {
		return nil, err
	}
	var metricsList []string
	for _, v := range matchRules {
		metricsList = append(metricsList, fmt.Sprintf("%s 0\n", v))
	}

	metricsText := strings.Join(metricsList, "")

	var parser expfmt.TextParser
	metricFamilies, err := parser.TextToMetricFamilies(strings.NewReader(metricsText))
	if err != nil {
		return nil, fmt.Errorf("couldn't parse match rules: %s", err)
	}
	return metricFamilies, nil
}

func parseSerializationFormat(value string) (Serializer, error) {
	switch value {
	case "json":
		return NewJSONSerializer()
	case "avro-json":
		return NewAvroJSONSerializer("schemas/metric.avsc")
	default:
		defaultTelemetry.Logger.Warn("invalid serialization format, using json", "serialization-format-value", value)
		return NewJSONSerializer()
	}
}

func parseTopicTemplate(tpl string) (*template.Template, error) {
	funcMap := template.FuncMap{
		"replace": func(old, new, src string) string {
			return strings.Replace(src, old, new, -1)
		},
		"substring": func(start, end int, s string) string {
			if start < 0 {
				start = 0
			}
			if end < 0 || end > len(s) {
				end = len(s)
			}
			if start >= end {
				panic("template function - substring: start is bigger (or equal) than end. That will produce an empty string.")
			}
			return s[start:end]
		},
	}
	return template.New("topic").Funcs(funcMap).Parse(tpl)
}
