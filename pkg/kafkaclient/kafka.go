package kafkaclient

import (
	"context"
	"errors"
	"fmt"
	"math"
	"stream-metrics-route/pkg/setting"
	"strings"
	"sync"
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

	mu    sync.RWMutex
	stats KafkaStats
}

type KafkaStats struct {
	SuccessCount   int64
	FailureCount  int64
	LastSuccessAt time.Time
	LastFailureAt time.Time
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
	writer := kafka.NewWriter(k.config)
	writer.Compression = k.compression
	k.Producer = writer
}

func (k *KafkaClient) Store(ctx context.Context, req []prompb.TimeSeries) error {
	metricsPerTopic, err := processWriteRequest(k.name, k.TopicTemplate, k.match, req)
	if err != nil {
		k.recordFailure()
		return fmt.Errorf("couldn't process write request %v", err)
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

		var lastErr error
		var backoff time.Duration
		const maxRetries = 3
		const initialBackoff = 100 * time.Millisecond
		const maxBackoff = 5 * time.Second

		for i := 0; i <= maxRetries; i++ {
			if i > 0 {
				select {
				case <-ctx.Done():
					k.recordFailure()
					return ctx.Err()
				case <-time.After(backoff):
				}
				backoff = time.Duration(math.Min(float64(backoff*2), float64(maxBackoff)))
			}

			writeCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
			err = k.Producer.WriteMessages(writeCtx, messages...)
			cancel()

			if err == nil {
				break
			}

			lastErr = err

			if errors.Is(err, kafka.LeaderNotAvailable) || errors.Is(err, context.DeadlineExceeded) {
				continue
			}
			if errors.Is(err, kafka.Unknown) {
				k.newWriter()
				continue
			}

			break
		}

		if err != nil {
			k.recordFailure()
			objectsFailed.WithLabelValues(k.name).Add(float64(len(messages)))
			return fmt.Errorf("kafka write failed after retries: %v", lastErr)
		}
	}

	k.recordSuccess()
	return nil
}

func (k *KafkaClient) recordSuccess() {
	k.mu.Lock()
	defer k.mu.Unlock()
	k.stats.SuccessCount++
	k.stats.LastSuccessAt = time.Now()
}

func (k *KafkaClient) recordFailure() {
	k.mu.Lock()
	defer k.mu.Unlock()
	k.stats.FailureCount++
	k.stats.LastFailureAt = time.Now()
}

func (k *KafkaClient) IsHealthy() bool {
	return true
}

func (k *KafkaClient) GetStats() map[string]interface{} {
	k.mu.RLock()
	defer k.mu.RUnlock()

	return map[string]interface{}{
		"name":            k.name,
		"success_count":   k.stats.SuccessCount,
		"failure_count":   k.stats.FailureCount,
		"last_success_at": k.stats.LastSuccessAt.Format(time.RFC3339),
		"last_failure_at": k.stats.LastFailureAt.Format(time.RFC3339),
	}
}
