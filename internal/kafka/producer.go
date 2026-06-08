// Package kafka содержит продюсер и консьюмер сообщений поверх segmentio/kafka-go.
package kafka

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/segmentio/kafka-go"

	"tea-platform/pkg/events"
)

// Producer публикует события в Kafka. Партиционирование — по AggregateID
// (Hash-балансер), что гарантирует порядок событий одного агрегата.
type Producer struct {
	writer *kafka.Writer
}

// NewProducer создаёт продюсера для списка брокеров (через запятую).
func NewProducer(brokers string) (*Producer, error) {
	writer := &kafka.Writer{
		Addr:                   kafka.TCP(strings.Split(brokers, ",")...),
		Balancer:               &kafka.Hash{},
		RequiredAcks:           kafka.RequireAll,
		AllowAutoTopicCreation: true,
	}
	return &Producer{writer: writer}, nil
}

// Publish сериализует конверт и отправляет его в топик с ключом = AggregateID.
func (p *Producer) Publish(ctx context.Context, topic string, env events.Envelope) error {
	value, err := json.Marshal(env)
	if err != nil {
		return err
	}
	return p.PublishRaw(ctx, topic, env.AggregateID, value)
}

// PublishRaw отправляет уже сериализованное сообщение. Используется relay-воркером
// outbox, который хранит готовый конверт в БД.
func (p *Producer) PublishRaw(ctx context.Context, topic, key string, value []byte) error {
	return p.writer.WriteMessages(ctx, kafka.Message{
		Topic: topic,
		Key:   []byte(key),
		Value: value,
	})
}

// Close закрывает продюсера.
func (p *Producer) Close() error {
	return p.writer.Close()
}
