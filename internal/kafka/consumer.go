package kafka

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"strings"

	"github.com/segmentio/kafka-go"

	"tea-platform/pkg/events"
)

// HandlerFunc обрабатывает одно событие. Возврат ошибки означает, что
// сообщение не будет закоммичено и будет повторно доставлено.
type HandlerFunc func(ctx context.Context, env events.Envelope) error

// Consumer читает события из топика в составе consumer-группы и передаёт их
// обработчику. Коммит смещения — только после успешной обработки (at-least-once).
type Consumer struct {
	reader *kafka.Reader
	log    *slog.Logger
}

// NewConsumer создаёт консьюмера для топика и группы.
func NewConsumer(brokers, topic, group string, log *slog.Logger) *Consumer {
	reader := kafka.NewReader(kafka.ReaderConfig{
		Brokers: strings.Split(brokers, ","),
		Topic:   topic,
		GroupID: group,
	})
	return &Consumer{reader: reader, log: log}
}

// Run читает сообщения до отмены ctx. «Ядовитые» сообщения (нечитаемый JSON)
// логируются и пропускаются, чтобы не блокировать партицию.
func (c *Consumer) Run(ctx context.Context, handler HandlerFunc) error {
	for {
		msg, err := c.reader.FetchMessage(ctx)
		if err != nil {
			if errors.Is(err, context.Canceled) {
				return nil
			}
			return err
		}

		var env events.Envelope
		if err := json.Unmarshal(msg.Value, &env); err != nil {
			c.log.Error("неразбираемое сообщение, пропускаю",
				"topic", msg.Topic, "offset", msg.Offset, "error", err)
			c.commit(ctx, msg)
			continue
		}

		if err := handler(ctx, env); err != nil {
			// Не коммитим — сообщение будет доставлено повторно.
			c.log.Error("обработчик вернул ошибку, будет повтор",
				"event_type", env.EventType, "event_id", env.EventID, "error", err)
			continue
		}

		c.commit(ctx, msg)
	}
}

func (c *Consumer) commit(ctx context.Context, msg kafka.Message) {
	if err := c.reader.CommitMessages(ctx, msg); err != nil {
		c.log.Error("не удалось закоммитить offset", "offset", msg.Offset, "error", err)
	}
}

// Close закрывает консьюмера.
func (c *Consumer) Close() error {
	return c.reader.Close()
}
