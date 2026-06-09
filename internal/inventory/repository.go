// Package inventory реализует Inventory Service: резервирование остатков по
// событию order.created с эмиссией stock.reserved / reservation.failed.
package inventory

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Repository — доступ к остаткам и резервам.
type Repository struct {
	pool *pgxpool.Pool
}

// NewRepository создаёт репозиторий инвентаря.
func NewRepository(pool *pgxpool.Pool) *Repository {
	return &Repository{pool: pool}
}

// HasReservations сообщает, есть ли уже резервы под заказ (идемпотентность
// при повторной доставке order.created).
func (r *Repository) HasReservations(ctx context.Context, orderID string) (bool, error) {
	var exists bool
	err := r.pool.QueryRow(ctx,
		`SELECT EXISTS(SELECT 1 FROM reservations WHERE order_id = $1)`, orderID,
	).Scan(&exists)
	return exists, err
}

// Commit окончательно фиксирует резервы заказа (reserved → committed).
// Остаток уже списан при резерве, поэтому меняется только статус. Идемпотентно.
func (r *Repository) Commit(ctx context.Context, orderID string) (int64, error) {
	ct, err := r.pool.Exec(ctx,
		`UPDATE reservations SET status = 'committed' WHERE order_id = $1 AND status = 'reserved'`,
		orderID,
	)
	if err != nil {
		return 0, err
	}
	return ct.RowsAffected(), nil
}

// Release освобождает резервы заказа (компенсация): возвращает остаток на склад
// и помечает резервы released. Идемпотентно — действует только на 'reserved'.
func (r *Repository) Release(ctx context.Context, orderID string) (int64, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return 0, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	rows, err := tx.Query(ctx,
		`SELECT product_id, quantity FROM reservations
		 WHERE order_id = $1 AND status = 'reserved' FOR UPDATE`,
		orderID,
	)
	if err != nil {
		return 0, err
	}

	type item struct {
		productID string
		qty       int32
	}
	var items []item
	for rows.Next() {
		var it item
		if err := rows.Scan(&it.productID, &it.qty); err != nil {
			rows.Close()
			return 0, err
		}
		items = append(items, it)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return 0, err
	}

	// Возвращаем остаток (для позиций с ограниченным запасом — чай).
	for _, it := range items {
		if _, err := tx.Exec(ctx,
			`UPDATE inventory SET available_qty = available_qty + $2, updated_at = now()
			 WHERE product_id = $1 AND available_qty IS NOT NULL`,
			it.productID, it.qty,
		); err != nil {
			return 0, err
		}
	}

	ct, err := tx.Exec(ctx,
		`UPDATE reservations SET status = 'released' WHERE order_id = $1 AND status = 'reserved'`,
		orderID,
	)
	if err != nil {
		return 0, err
	}
	if err := tx.Commit(ctx); err != nil {
		return 0, err
	}
	return ct.RowsAffected(), nil
}

// reserveItem пытается зарезервировать одну позицию в рамках транзакции.
// Возвращает ok=false при бизнес-отказе (неизвестный товар, стоп-лист,
// нехватка остатка); err — только при инфраструктурной ошибке.
func reserveItem(ctx context.Context, tx pgx.Tx, orderID, productID string, qty int32) (bool, error) {
	var (
		kind       string
		availQty   *int64 // NULL = неограниченно (блюда)
		inStoplist bool
	)
	err := tx.QueryRow(ctx,
		`SELECT kind, available_qty, in_stoplist FROM inventory WHERE product_id = $1 FOR UPDATE`,
		productID,
	).Scan(&kind, &availQty, &inStoplist)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil // товара нет в инвентаре — зарезервировать нельзя
	}
	if err != nil {
		return false, err
	}
	if inStoplist {
		return false, nil
	}

	// Ограниченный остаток (чай): проверяем и списываем.
	if availQty != nil {
		if *availQty < int64(qty) {
			return false, nil
		}
		if _, err := tx.Exec(ctx,
			`UPDATE inventory SET available_qty = available_qty - $2, updated_at = now() WHERE product_id = $1`,
			productID, qty,
		); err != nil {
			return false, err
		}
	}

	if _, err := tx.Exec(ctx,
		`INSERT INTO reservations (order_id, product_id, quantity, status)
		 VALUES ($1, $2, $3, 'reserved')
		 ON CONFLICT (order_id, product_id) DO NOTHING`,
		orderID, productID, qty,
	); err != nil {
		return false, err
	}
	return true, nil
}
