package kafka

import (
	"context"
	"errors"
	"strings"

	"github.com/segmentio/kafka-go"
)

// EnsureTopics идемпотентно создаёт топики (1 партиция, RF=1 — конфигурация
// одиночного брокера для разработки). Вызывается при старте сервиса до запуска
// консьюмеров, чтобы исключить гонку «консьюмер стартовал раньше, чем создан
// совсем новый топик». Уже существующие топики не считаются ошибкой.
func EnsureTopics(ctx context.Context, brokers string, topics ...string) error {
	client := &kafka.Client{Addr: kafka.TCP(strings.Split(brokers, ",")...)}

	configs := make([]kafka.TopicConfig, 0, len(topics))
	for _, t := range topics {
		configs = append(configs, kafka.TopicConfig{
			Topic:             t,
			NumPartitions:     1,
			ReplicationFactor: 1,
		})
	}

	resp, err := client.CreateTopics(ctx, &kafka.CreateTopicsRequest{Topics: configs})
	if err != nil {
		return err
	}

	// Игнорируем «топик уже существует», возвращаем первую реальную ошибку.
	for topic, topicErr := range resp.Errors {
		if topicErr != nil && !errors.Is(topicErr, kafka.TopicAlreadyExists) {
			return &topicCreateError{topic: topic, err: topicErr}
		}
	}
	return nil
}

type topicCreateError struct {
	topic string
	err   error
}

func (e *topicCreateError) Error() string {
	return "kafka: создание топика " + e.topic + ": " + e.err.Error()
}

func (e *topicCreateError) Unwrap() error { return e.err }
