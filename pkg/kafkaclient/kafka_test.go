package kafkaclient_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/segmentio/kafka-go"
	"github.com/segmentio/kafka-go/compress"
)

func TestKafkaStore(t *testing.T) {
	writer := &kafka.Writer{
		Addr: kafka.TCP(
			"192.168.11.119:9092", "192.168.11.164:9092", "192.168.11.15:9092",
		),
		Topic:                  "prometheus_monitor",
		Balancer:               &kafka.LeastBytes{},
		Compression:            compress.None,
		AllowAutoTopicCreation: true,
	}
	messages := []kafka.Message{
		{
			Key:   []byte("Key-A"),
			Value: []byte("Hello World!"),
		},
		{
			Key:   []byte("Key-B"),
			Value: []byte("One!"),
		},
		{
			Key:   []byte("Key-C"),
			Value: []byte("Two!"),
		},
	}

	var err error
	const retries = 3
	for i := 0; i < retries; i++ {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		// attempt to create topic prior to publishing the message
		err = writer.WriteMessages(ctx, messages...)
		if errors.Is(err, kafka.LeaderNotAvailable) || errors.Is(err, context.DeadlineExceeded) {
			time.Sleep(time.Millisecond * 250)
			continue
		}

		if err != nil {
			t.Errorf("unexpected error %v", err)
		}
		break
	}

	if err := writer.Close(); err != nil {
		t.Error("failed to close writer:", err)
	}
}
