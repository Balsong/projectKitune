package outbox

import (
	"context"
	"log/slog"
	"time"
)

// Publisher публикует сырое сообщение в топик (реализуется kafka.Producer).
type Publisher interface {
	PublishRaw(ctx context.Context, topic, key string, value []byte) error
}

// Relay периодически вычитывает неопубликованные события из outbox и шлёт их
// в Kafka. После успешной публикации помечает событие отправленным.
type Relay struct {
	repo      *Repo
	publisher Publisher
	log       *slog.Logger
	source    string
	interval  time.Duration
	batchSize int
}

// NewRelay создаёт relay-воркер, публикующий события одного источника source.
func NewRelay(repo *Repo, publisher Publisher, log *slog.Logger, source string) *Relay {
	return &Relay{
		repo:      repo,
		publisher: publisher,
		log:       log,
		source:    source,
		interval:  time.Second,
		batchSize: 100,
	}
}

// Run крутит цикл публикации до отмены ctx.
func (r *Relay) Run(ctx context.Context) {
	ticker := time.NewTicker(r.interval)
	defer ticker.Stop()

	r.log.Info("outbox relay запущен", "source", r.source, "interval", r.interval.String())
	for {
		select {
		case <-ctx.Done():
			r.log.Info("outbox relay остановлен")
			return
		case <-ticker.C:
			r.drain(ctx)
		}
	}
}

// drain публикует одну пачку неопубликованных событий.
func (r *Relay) drain(ctx context.Context) {
	msgs, err := r.repo.FetchUnpublished(ctx, r.source, r.batchSize)
	if err != nil {
		r.log.Error("relay: не удалось выбрать события", "error", err)
		return
	}

	for _, m := range msgs {
		if err := r.publisher.PublishRaw(ctx, m.Topic, m.AggregateID, m.Payload); err != nil {
			r.log.Error("relay: ошибка публикации", "id", m.ID, "topic", m.Topic, "error", err)
			_ = r.repo.IncAttempts(ctx, m.ID)
			// Прерываем пачку: сохраняем порядок и не плодим ретраи.
			return
		}
		if err := r.repo.MarkPublished(ctx, m.ID); err != nil {
			r.log.Error("relay: не удалось пометить событие отправленным", "id", m.ID, "error", err)
			return
		}
	}
}
