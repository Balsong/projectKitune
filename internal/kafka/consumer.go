package kafka

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"strings"
	"time"

	"github.com/segmentio/kafka-go"

	"tea-platform/internal/metrics"
	"tea-platform/pkg/events"
)

// HandlerFunc обрабатывает одно событие. Возврат ошибки означает повтор
// обработки; после исчерпания попыток сообщение уходит в DLQ.
type HandlerFunc func(ctx context.Context, env events.Envelope) error

const (
	// defaultMaxAttempts — сколько раз пытаемся обработать сообщение до DLQ.
	defaultMaxAttempts = 5
	// defaultBackoff — базовая задержка между попытками (растёт линейно).
	defaultBackoff = 500 * time.Millisecond
)

// Consumer читает события из топика в составе consumer-группы и передаёт их
// обработчику. Коммит смещения — только после успешной обработки или отправки
// в DLQ (at-least-once). «Ядовитые» сообщения (нечитаемый JSON или стабильно
// падающий обработчик) не блокируют партицию: после N попыток они уходят в
// dead-letter-топик «<topic>.dlq» и коммитятся.
type Consumer struct {
	reader      *kafka.Reader
	dlq         *Producer
	topic       string
	dlqTopic    string
	maxAttempts int
	backoff     time.Duration
	log         *slog.Logger
}

// NewConsumer создаёт консьюмера для топика и группы. DLQ-продюсер создаётся
// поверх тех же брокеров; при ошибке DLQ отключается (сообщения только
// логируются и коммитятся, чтобы не зациклиться).
func NewConsumer(brokers, topic, group string, log *slog.Logger) *Consumer {
	reader := kafka.NewReader(kafka.ReaderConfig{
		Brokers: strings.Split(brokers, ","),
		Topic:   topic,
		GroupID: group,
	})
	dlq, err := NewProducer(brokers)
	if err != nil {
		log.Warn("DLQ-продюсер не создан, dead-letter отключён", "topic", topic, "error", err)
		dlq = nil
	}
	dlqTopic := topic + ".dlq"
	// Создаём DLQ-топик заранее: авто-создание при первой публикации
	// у kafka-go гонит метаданные и теряет первое сообщение.
	if dlq != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		if err := EnsureTopics(ctx, brokers, dlqTopic); err != nil {
			log.Warn("не удалось создать DLQ-топик заранее", "dlq_topic", dlqTopic, "error", err)
		}
		cancel()
	}
	return &Consumer{
		reader:      reader,
		dlq:         dlq,
		topic:       topic,
		dlqTopic:    dlqTopic,
		maxAttempts: defaultMaxAttempts,
		backoff:     defaultBackoff,
		log:         log,
	}
}

// Run читает сообщения до отмены ctx (graceful shutdown).
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
			c.log.Error("неразбираемое сообщение, в DLQ",
				"topic", msg.Topic, "offset", msg.Offset, "error", err)
			c.deadLetter(msg, "parse", err.Error())
			c.commit(ctx, msg)
			continue
		}

		if err := c.handleWithRetries(ctx, handler, env); err != nil {
			// Останов во время обработки — не коммитим, дадим переобработать.
			if ctx.Err() != nil {
				return nil
			}
			c.log.Error("исчерпаны попытки обработки, в DLQ",
				"event_type", env.EventType, "event_id", env.EventID,
				"attempts", c.maxAttempts, "error", err)
			c.deadLetter(msg, "handler", err.Error())
			c.commit(ctx, msg)
			continue
		}

		metrics.EventsConsumed.WithLabelValues(c.topic).Inc()
		c.commit(ctx, msg)
	}
}

// handleWithRetries вызывает обработчик с линейным backoff до maxAttempts.
// Прерывается при отмене ctx, чтобы не жечь попытки на остановке.
func (c *Consumer) handleWithRetries(ctx context.Context, handler HandlerFunc, env events.Envelope) error {
	var err error
	for attempt := 1; attempt <= c.maxAttempts; attempt++ {
		if err = handler(ctx, env); err == nil {
			return nil
		}
		if ctx.Err() != nil {
			return err
		}
		c.log.Warn("обработчик вернул ошибку, повтор",
			"event_type", env.EventType, "event_id", env.EventID,
			"attempt", attempt, "max", c.maxAttempts, "error", err)
		if attempt < c.maxAttempts {
			metrics.EventRetries.WithLabelValues(c.topic).Inc()
			select {
			case <-ctx.Done():
				return err
			case <-time.After(c.backoff * time.Duration(attempt)):
			}
		}
	}
	return err
}

// dlqEnvelope — обёртка DLQ: исходное сообщение + причина и текст ошибки.
// Payload используется для валидного JSON, RawPayload — для нечитаемых байт
// (parse-ошибки), чтобы сама обёртка всегда оставалась корректным JSON.
type dlqEnvelope struct {
	OriginalTopic string          `json:"original_topic"`
	Reason        string          `json:"reason"` // parse | handler
	Error         string          `json:"error"`  // текст последней ошибки
	Attempts      int             `json:"attempts"`
	FailedAt      string          `json:"failed_at"`
	Payload       json.RawMessage `json:"payload,omitempty"`     // исходный конверт (валидный JSON)
	RawPayload    string          `json:"raw_payload,omitempty"` // нечитаемые байты как строка
}

// deadLetter публикует «ядовитое» сообщение в DLQ-топик. Использует фоновый
// контекст с таймаутом, чтобы не потерять сообщение при остановке консьюмера.
func (c *Consumer) deadLetter(msg kafka.Message, reason, errText string) {
	metrics.EventsDeadLettered.WithLabelValues(c.topic, reason).Inc()
	if c.dlq == nil {
		return
	}
	wrapper := dlqEnvelope{
		OriginalTopic: c.topic,
		Reason:        reason,
		Error:         errText,
		Attempts:      c.maxAttempts,
		FailedAt:      time.Now().UTC().Format(time.RFC3339),
	}
	if json.Valid(msg.Value) {
		wrapper.Payload = json.RawMessage(msg.Value)
	} else {
		wrapper.RawPayload = string(msg.Value)
	}
	value, err := json.Marshal(wrapper)
	if err != nil {
		value = msg.Value // фолбэк: исходные байты как есть
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := c.dlq.PublishRaw(ctx, c.dlqTopic, string(msg.Key), value); err != nil {
		c.log.Error("не удалось отправить в DLQ", "dlq_topic", c.dlqTopic, "error", err)
	}
}

func (c *Consumer) commit(ctx context.Context, msg kafka.Message) {
	if err := c.reader.CommitMessages(ctx, msg); err != nil {
		c.log.Error("не удалось закоммитить offset", "offset", msg.Offset, "error", err)
	}
}

// Close закрывает консьюмера и DLQ-продюсера.
func (c *Consumer) Close() error {
	if c.dlq != nil {
		_ = c.dlq.Close()
	}
	return c.reader.Close()
}
