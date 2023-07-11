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

	"github.com/prometheus/prometheus/prompb"
	"github.com/segmentio/kafka-go"
	"github.com/segmentio/kafka-go/compress"
)

var (
	defaultBatchSize        = 1000
	defaultBatchBytes int64 = 1048576
)

type KafkaClient struct {
	name          string
	Producer      *kafka.Writer
	TopicTemplate template.Template
}

func NewKafka(name string, cfg setting.KafkaConfig) (*KafkaClient, error) {
	topicTemplate, err := parseTopicTemplate(cfg.KafkaTopic)
	if err != nil {
		return nil, fmt.Errorf("couldn't parse the topic template", err)
	}
	matchList, err := parseMatchList(cfg.Match)
	if err != nil {
		return nil, fmt.Errorf("couldn't parse the match rules", err)
	}
	match = matchList
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

	if cfg.KafkaBatchNumMessages < 0 {
		cfg.KafkaBatchNumMessages = defaultBatchSize
	}
	if cfg.KafkaBatchBytes < 0 {
		cfg.KafkaBatchBytes = defaultBatchBytes
	}

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
	async := false
	if cfg.Async {
		async = cfg.Async
	}

	writer := &kafka.Writer{
		Addr: kafka.TCP(
			strings.Split(cfg.KafkaBrokerList, ",")...,
		),
		Topic:       cfg.KafkaTopic,
		BatchSize:   cfg.KafkaBatchNumMessages,
		BatchBytes:  cfg.KafkaBatchBytes,
		Compression: compression,
		Balancer:    balancer,
		Async:       async,
	}
	return &KafkaClient{
		name:          name,
		Producer:      writer,
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
				defaultTelemetry.Logger.Error("unexpected error", err)
			}
			break
		}

		if err := k.Producer.Close(); err != nil {
			defaultTelemetry.Logger.Error("failed to close writer", err)
		}
	}
	return 0, nil
}
