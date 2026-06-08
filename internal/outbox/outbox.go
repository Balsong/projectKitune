// Package outbox реализует паттерн transactional outbox: события пишутся в БД
// в одной транзакции с бизнес-данными, а relay-воркер асинхронно публикует их
// в Kafka. Это исключает потерю событий между commit и publish.
package outbox

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"tea-platform/pkg/events"
)

// Repo — доступ к таблице outbox.
type Repo struct {
	pool *pgxpool.Pool
}

// NewRepo создаёт репозиторий outbox.
func NewRepo(pool *pgxpool.Pool) *Repo {
	return &Repo{pool: pool}
}

// Message — неопубликованная запись outbox для relay.
type Message struct {
	ID          int64
	AggregateID string
	Topic       string
	Payload     []byte
}

// SaveTx записывает событие в outbox в рамках переданной транзакции.
// Конверт сериализуется в JSON и хранится как payload.
func SaveTx(ctx context.Context, tx pgx.Tx, topic string, env events.Envelope) error {
	payload, err := json.Marshal(env)
	if err != nil {
		return fmt.Errorf("outbox: сериализация события: %w", err)
	}
	const q = `
		INSERT INTO outbox (aggregate_id, topic, event_type, payload)
		VALUES ($1, $2, $3, $4)`
	if _, err := tx.Exec(ctx, q, env.AggregateID, topic, env.EventType, payload); err != nil {
		return fmt.Errorf("outbox: вставка события: %w", err)
	}
	return nil
}

// FetchUnpublished возвращает до limit неопубликованных событий по возрастанию id.
func (r *Repo) FetchUnpublished(ctx context.Context, limit int) ([]Message, error) {
	const q = `
		SELECT id, aggregate_id, topic, payload
		FROM outbox
		WHERE published_at IS NULL
		ORDER BY id
		LIMIT $1`
	rows, err := r.pool.Query(ctx, q, limit)
	if err != nil {
		return nil, fmt.Errorf("outbox: выборка неопубликованных: %w", err)
	}
	defer rows.Close()

	var msgs []Message
	for rows.Next() {
		var m Message
		if err := rows.Scan(&m.ID, &m.AggregateID, &m.Topic, &m.Payload); err != nil {
			return nil, fmt.Errorf("outbox: чтение строки: %w", err)
		}
		msgs = append(msgs, m)
	}
	return msgs, rows.Err()
}

// MarkPublished помечает событие отправленным.
func (r *Repo) MarkPublished(ctx context.Context, id int64) error {
	const q = `UPDATE outbox SET published_at = now() WHERE id = $1`
	_, err := r.pool.Exec(ctx, q, id)
	return err
}

// IncAttempts увеличивает счётчик попыток публикации (для диагностики).
func (r *Repo) IncAttempts(ctx context.Context, id int64) error {
	const q = `UPDATE outbox SET attempts = attempts + 1 WHERE id = $1`
	_, err := r.pool.Exec(ctx, q, id)
	return err
}
