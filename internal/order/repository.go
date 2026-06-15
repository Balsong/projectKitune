package order

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"tea-platform/internal/outbox"
	"tea-platform/pkg/events"
)

// Repository — доступ к заказам в PostgreSQL.
type Repository struct {
	pool *pgxpool.Pool
}

// NewRepository создаёт репозиторий заказов.
func NewRepository(pool *pgxpool.Pool) *Repository {
	return &Repository{pool: pool}
}

// Create сохраняет заказ, его позиции и событие order.created в outbox —
// всё в одной транзакции (transactional outbox). Заполняет o.ID и o.CreatedAt.
func (r *Repository) Create(ctx context.Context, o *Order) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("order: begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }() // no-op после Commit

	const insertOrder = `
		INSERT INTO orders (user_id, cart_id, fulfillment_type, status, total_cents, currency, address, booking_id)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		RETURNING id, created_at`
	if err := tx.QueryRow(ctx, insertOrder,
		o.UserID, o.CartID, o.FulfillmentType, o.Status, o.TotalCents, o.Currency, o.Address, o.BookingID,
	).Scan(&o.ID, &o.CreatedAt); err != nil {
		return fmt.Errorf("order: insert order: %w", err)
	}

	const insertItem = `
		INSERT INTO order_items (order_id, product_id, name, quantity, unit_price_cents, subtotal_cents)
		VALUES ($1, $2, $3, $4, $5, $6)`
	for _, it := range o.Items {
		if _, err := tx.Exec(ctx, insertItem,
			o.ID, it.ProductID, it.Name, it.Quantity, it.UnitPriceCents, it.SubtotalCents,
		); err != nil {
			return fmt.Errorf("order: insert item: %w", err)
		}
	}

	// Событие order.created в той же транзакции — гарантия отсутствия потерь.
	env, err := events.New(events.EventOrderCreated, 1, o.ID, OrderCreatedPayload{
		OrderID:         o.ID,
		UserID:          o.UserID,
		FulfillmentType: o.FulfillmentType,
		Items:           o.Items,
		TotalCents:      o.TotalCents,
		Currency:        o.Currency,
	})
	if err != nil {
		return fmt.Errorf("order: build event: %w", err)
	}
	if err := outbox.SaveTx(ctx, tx, outbox.SourceOrder, events.TopicOrders, env); err != nil {
		return err
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("order: commit: %w", err)
	}
	return nil
}

// EmitSpec описывает событие, публикуемое в outbox в одной транзакции с
// переходом состояния заказа.
type EmitSpec struct {
	Source string
	Topic  string
	Env    events.Envelope
}

// Transition атомарно переводит заказ в статус to, если текущий статус —
// один из from (CAS), и, если задан emit, пишет событие в outbox в той же
// транзакции. Несколько допустимых from-статусов делают сагу устойчивой к
// порядку событий (напр. payment.succeeded может прийти раньше, чем
// stock.reserved успел перевести заказ в payment_pending). applied=false —
// статус не совпал: переход уже выполнен или пришёл не вовремя (идемпотентность).
func (r *Repository) Transition(ctx context.Context, orderID string, from []string, to string, emit *EmitSpec) (bool, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return false, fmt.Errorf("order: begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	ct, err := tx.Exec(ctx,
		`UPDATE orders SET status = $2, updated_at = now() WHERE id = $1 AND status = ANY($3)`,
		orderID, to, from,
	)
	if err != nil {
		return false, fmt.Errorf("order: update status: %w", err)
	}
	if ct.RowsAffected() == 0 {
		return false, nil // статус не совпал — пропускаем
	}

	if emit != nil {
		if err := outbox.SaveTx(ctx, tx, emit.Source, emit.Topic, emit.Env); err != nil {
			return false, err
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return false, fmt.Errorf("order: commit: %w", err)
	}
	return true, nil
}

// Get возвращает заказ с позициями или ErrNotFound.
func (r *Repository) Get(ctx context.Context, id string) (*Order, error) {
	const selectOrder = `
		SELECT id, user_id, cart_id, fulfillment_type, status, total_cents, currency, address, booking_id, created_at
		FROM orders WHERE id = $1`

	var o Order
	err := r.pool.QueryRow(ctx, selectOrder, id).Scan(
		&o.ID, &o.UserID, &o.CartID, &o.FulfillmentType, &o.Status,
		&o.TotalCents, &o.Currency, &o.Address, &o.BookingID, &o.CreatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("order: select order: %w", err)
	}

	const selectItems = `
		SELECT product_id, name, quantity, unit_price_cents, subtotal_cents
		FROM order_items WHERE order_id = $1 ORDER BY id`
	rows, err := r.pool.Query(ctx, selectItems, id)
	if err != nil {
		return nil, fmt.Errorf("order: select items: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var it Item
		if err := rows.Scan(&it.ProductID, &it.Name, &it.Quantity, &it.UnitPriceCents, &it.SubtotalCents); err != nil {
			return nil, fmt.Errorf("order: scan item: %w", err)
		}
		o.Items = append(o.Items, it)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return &o, nil
}
