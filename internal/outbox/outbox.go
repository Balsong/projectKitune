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

// Источники событий (колонка source). Каждый relay публикует только свои.
const (
	SourceOrder     = "order-svc"
	SourceInventory = "inventory-svc"
	SourcePayment   = "payment-svc"
	SourceDelivery  = "delivery-svc"
	SourceBooking   = "booking-svc"
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
// source — сервис-источник, по нему relay отбирает свои события.
func SaveTx(ctx context.Context, tx pgx.Tx, source, topic string, env events.Envelope) error {
	payload, err := json.Marshal(env)
	if err != nil {
		return fmt.Errorf("outbox: сериализация события: %w", err)
	}
	const q = `
		INSERT INTO outbox (source, aggregate_id, topic, event_type, payload)
		VALUES ($1, $2, $3, $4, $5)`
	if _, err := tx.Exec(ctx, q, source, env.AggregateID, topic, env.EventType, payload); err != nil {
		return fmt.Errorf("outbox: вставка события: %w", err)
	}
	return nil
}

// Save записывает событие в outbox в собственной транзакции. Используется,
// когда нет бизнес-транзакции, к которой можно присоединиться (напр. событие
// reservation.failed, при котором резерв откатывается).
func (r *Repo) Save(ctx context.Context, source, topic string, env events.Envelope) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := SaveTx(ctx, tx, source, topic, env); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// FetchUnpublished возвращает до limit неопубликованных событий источника
// source по возрастанию id.
func (r *Repo) FetchUnpublished(ctx context.Context, source string, limit int) ([]Message, error) {
	const q = `
		SELECT id, aggregate_id, topic, payload
		FROM outbox
		WHERE published_at IS NULL AND source = $1
		ORDER BY id
		LIMIT $2`
	rows, err := r.pool.Query(ctx, q, source, limit)
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
