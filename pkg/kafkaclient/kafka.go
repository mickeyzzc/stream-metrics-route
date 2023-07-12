package kafkaclient

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"stream-metrics-route/pkg/setting"
	"strings"
	"text/template"
	"time"

	dto "github.com/prometheus/client_model/go"
	"github.com/prometheus/prometheus/prompb"
	"github.com/segmentio/kafka-go"
	"github.com/segmentio/kafka-go/compress"
)

var (
	defaultBatchSize  = 1000
	defaultBatchBytes = 1048576
)

type KafkaClient struct {
	name          string
	match         map[string]*dto.MetricFamily
	compression   compress.Compression
	Producer      *kafka.Writer
	TopicTemplate template.Template
	config        kafka.WriterConfig
}

func NewKafka(name string, cfg setting.KafkaConfig) (*KafkaClient, error) {

	topicTemplate, err := parseTopicTemplate(cfg.KafkaTopic)
	if err != nil {
		return nil, fmt.Errorf("couldn't parse the topic template %v", err)
	}

	matchList, err := parseMatchList(cfg.Match)
	if err != nil {
		return nil, fmt.Errorf("couldn't parse the match rules %v", err)
	}
	var kafkaClient = &KafkaClient{
		name:          name,
		match:         matchList,
		TopicTemplate: *topicTemplate,
	}
	kafkaClient.newWriterConfig(cfg)

	var compression compress.Compression
	switch cfg.KafkaCompression {
	case "snappy":
		compression = compress.Snappy
	case "gzip":
		compression = compress.Gzip
	case "lz4":
		compression = compress.Lz4
	case "zstd":
		compression = compress.Zstd
	default:
		compression = compress.None
	}

	brokers := strings.Split(cfg.KafkaBrokerList, ",")
	defaultTelemetry.Logger.Debug("create kafka client", "name", name, "brokers", brokers)
	kafkaClient.compression = compression
	kafkaClient.newWriter()
	return kafkaClient, nil
}

func (k *KafkaClient) newWriterConfig(cfg setting.KafkaConfig) {

	var balancer kafka.Balancer
	switch cfg.Balancer {
	case "crc32":
		balancer = &kafka.CRC32Balancer{}
	case "hash":
		balancer = &kafka.Hash{}
	case "roundrobin":
		balancer = &kafka.RoundRobin{}
	case "murmur2":
		balancer = &kafka.Murmur2Balancer{}
	case "referencehash":
		balancer = &kafka.ReferenceHash{}
	default:
		balancer = &kafka.LeastBytes{}
	}

	if cfg.KafkaBatchNumMessages <= 0 {
		cfg.KafkaBatchNumMessages = defaultBatchSize
	}
	if cfg.KafkaBatchBytes <= 0 {
		cfg.KafkaBatchBytes = defaultBatchBytes
	}

	async := false
	if cfg.Async {
		async = cfg.Async
	}
	brokers := strings.Split(cfg.KafkaBrokerList, ",")
	k.config = kafka.WriterConfig{
		Brokers:    brokers,
		Topic:      cfg.KafkaTopic,
		BatchSize:  cfg.KafkaBatchNumMessages,
		BatchBytes: cfg.KafkaBatchBytes,
		Balancer:   balancer,
		Async:      async,
	}
}

func (k *KafkaClient) newWriter() {
	defaultTelemetry.Logger.Info("create kafka writer", "name", k.name)
	writer := kafka.NewWriter(kafka.WriterConfig{})
	writer.Compression = k.compression
	k.Producer = writer
}

func (k *KafkaClient) Store(ctx context.Context, req []prompb.TimeSeries) (int, error) {
	defer ctx.Done()
	metricsPerTopic, err := processWriteRequest(k.name, k.TopicTemplate, k.match, req)
	if err != nil {
		return http.StatusInternalServerError, fmt.Errorf("couldn't process write request %v", err)
	}

	for topic, metrics := range metricsPerTopic {
		t := topic
		defaultTelemetry.Logger.Debug("write request", "name", k.name, "metricsPerTopic", t)
		messages := []kafka.Message{}
		for _, metric := range metrics {
			objectsWritten.WithLabelValues(k.name).Add(float64(1))
			messages = append(messages, kafka.Message{
				Value: metric,
			})
		}

		var err error
		const retries = 3
		for i := 0; i < retries; i++ {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()

			// attempt to create topic prior to publishing the message
			err = k.Producer.WriteMessages(ctx, messages...)
			if errors.Is(err, kafka.LeaderNotAvailable) || errors.Is(err, context.DeadlineExceeded) {
				time.Sleep(time.Millisecond * 250)
				continue
			}
			if err != nil {
				defaultTelemetry.Logger.Error("unexpected error", "errmsg", err)
				if errors.Is(err, kafka.Unknown) {
					k.newWriter()
					continue
				}

			}
			break
		}
		if err != nil {
			objectsFailed.WithLabelValues(k.name).Add(float64(len(messages)))
			return http.StatusInternalServerError, err
		}
		/*
			if err := k.Producer.Close(); err != nil {
				defaultTelemetry.Logger.Error("failed to close writer", "errmsg", err)
			}
		*/
	}
	return 0, nil
}
