package delivery

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Repository — доступ к доставкам.
type Repository struct {
	pool *pgxpool.Pool
}

// NewRepository создаёт репозиторий доставок.
func NewRepository(pool *pgxpool.Pool) *Repository {
	return &Repository{pool: pool}
}

// Exists сообщает, есть ли уже доставка по заказу (идемпотентность).
func (r *Repository) Exists(ctx context.Context, orderID string) (bool, error) {
	var exists bool
	err := r.pool.QueryRow(ctx,
		`SELECT EXISTS(SELECT 1 FROM deliveries WHERE order_id = $1)`, orderID,
	).Scan(&exists)
	return exists, err
}

// Get возвращает доставку по заказу или ErrNotFound.
func (r *Repository) Get(ctx context.Context, orderID string) (*Delivery, error) {
	const q = `
		SELECT order_id, fulfillment_type, status, tracking_code, courier, eta, created_at
		FROM deliveries WHERE order_id = $1`
	var d Delivery
	err := r.pool.QueryRow(ctx, q, orderID).Scan(
		&d.OrderID, &d.FulfillmentType, &d.Status, &d.TrackingCode, &d.Courier, &d.ETA, &d.CreatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("delivery: select: %w", err)
	}
	return &d, nil
}

// insertTx вставляет доставку в рамках транзакции.
func insertTx(ctx context.Context, tx pgx.Tx, d *Delivery) error {
	const q = `
		INSERT INTO deliveries (order_id, fulfillment_type, status, tracking_code, courier, eta, correlation_id)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		ON CONFLICT (order_id) DO NOTHING`
	_, err := tx.Exec(ctx, q, d.OrderID, d.FulfillmentType, d.Status, d.TrackingCode, d.Courier, d.ETA, d.CorrelationID)
	return err
}

// Dispatched — строка для симулятора курьера.
type Dispatched struct {
	OrderID       string
	CorrelationID string
}

// FetchDispatched возвращает доставки в статусе dispatched (для симулятора).
func (r *Repository) FetchDispatched(ctx context.Context, limit int) ([]Dispatched, error) {
	const q = `SELECT order_id, correlation_id FROM deliveries WHERE status = 'dispatched' ORDER BY updated_at LIMIT $1`
	rows, err := r.pool.Query(ctx, q, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Dispatched
	for rows.Next() {
		var d Dispatched
		if err := rows.Scan(&d.OrderID, &d.CorrelationID); err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

// markDeliveredTx переводит доставку dispatched → delivered в рамках транзакции.
func markDeliveredTx(ctx context.Context, tx pgx.Tx, orderID string) (bool, error) {
	ct, err := tx.Exec(ctx,
		`UPDATE deliveries SET status = 'delivered', updated_at = now()
		 WHERE order_id = $1 AND status = 'dispatched'`,
		orderID,
	)
	if err != nil {
		return false, err
	}
	return ct.RowsAffected() > 0, nil
}
