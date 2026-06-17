package kafka

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"
	"time"

	"tea-platform/pkg/events"
)

// newTestConsumer собирает консьюмера без реальных reader/DLQ для проверки
// чистой логики ретраев (handleWithRetries их не использует).
func newTestConsumer(maxAttempts int) *Consumer {
	return &Consumer{
		topic:       "test.events",
		dlqTopic:    "test.events.dlq",
		maxAttempts: maxAttempts,
		backoff:     time.Millisecond,
		log:         slog.New(slog.NewTextHandler(io.Discard, nil)),
	}
}

func TestHandleWithRetries_SucceedsAfterTransientFailures(t *testing.T) {
	c := newTestConsumer(5)
	calls := 0
	handler := func(_ context.Context, _ events.Envelope) error {
		calls++
		if calls < 3 {
			return errors.New("временная ошибка")
		}
		return nil
	}
	if err := c.handleWithRetries(context.Background(), handler, events.Envelope{}); err != nil {
		t.Fatalf("ожидали успех после ретраев, получили: %v", err)
	}
	if calls != 3 {
		t.Fatalf("ожидали 3 вызова обработчика, получили %d", calls)
	}
}

func TestHandleWithRetries_ExhaustsAndReturnsError(t *testing.T) {
	c := newTestConsumer(4)
	calls := 0
	wantErr := errors.New("стабильная ошибка")
	handler := func(_ context.Context, _ events.Envelope) error {
		calls++
		return wantErr
	}
	err := c.handleWithRetries(context.Background(), handler, events.Envelope{})
	if !errors.Is(err, wantErr) {
		t.Fatalf("ожидали проброс ошибки, получили: %v", err)
	}
	if calls != 4 {
		t.Fatalf("ожидали 4 попытки (maxAttempts), получили %d", calls)
	}
}

func TestHandleWithRetries_StopsOnContextCancel(t *testing.T) {
	c := newTestConsumer(10)
	ctx, cancel := context.WithCancel(context.Background())
	calls := 0
	handler := func(_ context.Context, _ events.Envelope) error {
		calls++
		cancel() // отмена после первой неудачи
		return errors.New("ошибка")
	}
	_ = c.handleWithRetries(ctx, handler, events.Envelope{})
	if calls != 1 {
		t.Fatalf("ожидали остановку после отмены ctx (1 вызов), получили %d", calls)
	}
}
