package shop

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ErrNotFound — товар или заказ не найден.
var ErrNotFound = errors.New("shop: not found")

// ErrOutOfStock — недостаточно остатка по позиции.
var ErrOutOfStock = errors.New("shop: out of stock")

// Repository — доступ к данным магазина в БД tea_shop.
type Repository struct {
	pool *pgxpool.Pool
}

// NewRepository создаёт репозиторий магазина.
func NewRepository(pool *pgxpool.Pool) *Repository {
	return &Repository{pool: pool}
}

const productColumns = `id, sku, name, description, category, price_cents, currency, weight_grams, unit, image_url, stock_qty, available`

func scanProduct(row pgx.Row) (Product, error) {
	var p Product
	err := row.Scan(&p.ID, &p.SKU, &p.Name, &p.Description, &p.Category,
		&p.PriceCents, &p.Currency, &p.WeightGrams, &p.Unit, &p.ImageURL, &p.StockQty, &p.Available)
	return p, err
}

// ListProducts возвращает товары с опциональным фильтром по категории.
func (r *Repository) ListProducts(ctx context.Context, category string) ([]Product, error) {
	const q = `SELECT ` + productColumns + `
		FROM products WHERE ($1 = '' OR category = $1) ORDER BY available DESC, name`
	rows, err := r.pool.Query(ctx, q, category)
	if err != nil {
		return nil, fmt.Errorf("shop: list products: %w", err)
	}
	defer rows.Close()
	var out []Product
	for rows.Next() {
		p, err := scanProduct(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// GetProduct возвращает товар по id или ErrNotFound.
func (r *Repository) GetProduct(ctx context.Context, id string) (Product, error) {
	const q = `SELECT ` + productColumns + ` FROM products WHERE id = $1`
	p, err := scanProduct(r.pool.QueryRow(ctx, q, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return Product{}, ErrNotFound
	}
	return p, err
}

// UpdateProduct меняет цену, остаток и доступность (админка).
func (r *Repository) UpdateProduct(ctx context.Context, id string, priceCents int64, stockQty int32, available bool) (Product, error) {
	const q = `UPDATE products SET price_cents = $2, stock_qty = $3, available = $4
		WHERE id = $1 RETURNING ` + productColumns
	p, err := scanProduct(r.pool.QueryRow(ctx, q, id, priceCents, stockQty, available))
	if errors.Is(err, pgx.ErrNoRows) {
		return Product{}, ErrNotFound
	}
	return p, err
}

// CartLines возвращает позиции корзины с актуальными данными товара.
func (r *Repository) CartLines(ctx context.Context, cartID string) ([]CartLine, error) {
	const q = `
		SELECT p.id, p.sku, p.name, ci.quantity, p.price_cents, p.stock_qty
		FROM cart_items ci JOIN products p ON p.id = ci.product_id
		WHERE ci.cart_id = $1 ORDER BY ci.added_at`
	rows, err := r.pool.Query(ctx, q, cartID)
	if err != nil {
		return nil, fmt.Errorf("shop: cart lines: %w", err)
	}
	defer rows.Close()
	var out []CartLine
	for rows.Next() {
		var l CartLine
		if err := rows.Scan(&l.ProductID, &l.SKU, &l.Name, &l.Quantity, &l.UnitPriceCents, &l.StockQty); err != nil {
			return nil, err
		}
		l.SubtotalCents = l.UnitPriceCents * int64(l.Quantity)
		out = append(out, l)
	}
	return out, rows.Err()
}

// AddToCart добавляет количество товара в корзину (clamp по остатку).
func (r *Repository) AddToCart(ctx context.Context, cartID, productID string, qty int32) error {
	if qty <= 0 {
		qty = 1
	}
	const q = `
		INSERT INTO cart_items (cart_id, product_id, quantity)
		VALUES ($1, $2, LEAST($3, (SELECT stock_qty FROM products WHERE id = $2)))
		ON CONFLICT (cart_id, product_id)
		DO UPDATE SET quantity = LEAST(
			cart_items.quantity + $3,
			(SELECT stock_qty FROM products WHERE id = $2))`
	tag, err := r.pool.Exec(ctx, q, cartID, productID, qty)
	if err != nil {
		return fmt.Errorf("shop: add to cart: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// RemoveFromCart уменьшает количество позиции, удаляя её при достижении нуля.
func (r *Repository) RemoveFromCart(ctx context.Context, cartID, productID string, qty int32) error {
	if qty <= 0 {
		qty = 1
	}
	const upd = `UPDATE cart_items SET quantity = quantity - $3
		WHERE cart_id = $1 AND product_id = $2 AND quantity > $3`
	tag, err := r.pool.Exec(ctx, upd, cartID, productID, qty)
	if err != nil {
		return fmt.Errorf("shop: remove from cart: %w", err)
	}
	if tag.RowsAffected() == 0 {
		// Количество <= qty — удаляем позицию целиком.
		_, err = r.pool.Exec(ctx, `DELETE FROM cart_items WHERE cart_id = $1 AND product_id = $2`, cartID, productID)
	}
	return err
}

// ClearCart очищает корзину.
func (r *Repository) ClearCart(ctx context.Context, cartID string) error {
	_, err := r.pool.Exec(ctx, `DELETE FROM cart_items WHERE cart_id = $1`, cartID)
	return err
}

// GetOrder возвращает заказ с позициями или ErrNotFound.
func (r *Repository) GetOrder(ctx context.Context, id string) (*Order, error) {
	const q = `SELECT id, user_id, status, customer, phone, email, country, region, city,
		address, postal_code, comment, items_subtotal_cents, delivery_cents, total_cents,
		tracking_code, created_at FROM orders WHERE id = $1`
	o, err := scanOrder(r.pool.QueryRow(ctx, q, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	items, err := r.orderItems(ctx, o.ID)
	if err != nil {
		return nil, err
	}
	o.Items = items
	return o, nil
}

// ListByUser возвращает заказы пользователя (новые первыми).
func (r *Repository) ListByUser(ctx context.Context, userID string, limit int) ([]*Order, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	const q = `SELECT id, user_id, status, customer, phone, email, country, region, city,
		address, postal_code, comment, items_subtotal_cents, delivery_cents, total_cents,
		tracking_code, created_at FROM orders WHERE user_id = $1 ORDER BY created_at DESC LIMIT $2`
	return r.queryOrders(ctx, q, userID, limit)
}

// ListAll возвращает все заказы (админка).
func (r *Repository) ListAll(ctx context.Context, limit int) ([]*Order, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	const q = `SELECT id, user_id, status, customer, phone, email, country, region, city,
		address, postal_code, comment, items_subtotal_cents, delivery_cents, total_cents,
		tracking_code, created_at FROM orders ORDER BY created_at DESC LIMIT $1`
	return r.queryOrders(ctx, q, limit)
}

func (r *Repository) queryOrders(ctx context.Context, q string, args ...any) ([]*Order, error) {
	rows, err := r.pool.Query(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("shop: query orders: %w", err)
	}
	defer rows.Close()
	var orders []*Order
	for rows.Next() {
		o, err := scanOrder(rows)
		if err != nil {
			return nil, err
		}
		orders = append(orders, o)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	for _, o := range orders {
		items, err := r.orderItems(ctx, o.ID)
		if err != nil {
			return nil, err
		}
		o.Items = items
	}
	return orders, nil
}

func scanOrder(row pgx.Row) (*Order, error) {
	var o Order
	err := row.Scan(&o.ID, &o.UserID, &o.Status, &o.Customer, &o.Phone, &o.Email,
		&o.Country, &o.Region, &o.City, &o.Address, &o.PostalCode, &o.Comment,
		&o.ItemsSubtotalCents, &o.DeliveryCents, &o.TotalCents, &o.TrackingCode, &o.CreatedAt)
	if err != nil {
		return nil, err
	}
	return &o, nil
}

func (r *Repository) orderItems(ctx context.Context, orderID string) ([]OrderLine, error) {
	const q = `SELECT product_id, name, quantity, unit_price_cents, subtotal_cents
		FROM order_items WHERE order_id = $1 ORDER BY id`
	rows, err := r.pool.Query(ctx, q, orderID)
	if err != nil {
		return nil, fmt.Errorf("shop: order items: %w", err)
	}
	defer rows.Close()
	var out []OrderLine
	for rows.Next() {
		var l OrderLine
		if err := rows.Scan(&l.ProductID, &l.Name, &l.Quantity, &l.UnitPriceCents, &l.SubtotalCents); err != nil {
			return nil, err
		}
		out = append(out, l)
	}
	return out, rows.Err()
}

// UpdateOrderStatus меняет статус заказа и трек-номер (админка).
func (r *Repository) UpdateOrderStatus(ctx context.Context, id, status, tracking string) (*Order, error) {
	const q = `UPDATE orders SET status = $2,
		tracking_code = CASE WHEN $3 <> '' THEN $3 ELSE tracking_code END,
		updated_at = now() WHERE id = $1`
	tag, err := r.pool.Exec(ctx, q, id, status, tracking)
	if err != nil {
		return nil, fmt.Errorf("shop: update order: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return nil, ErrNotFound
	}
	return r.GetOrder(ctx, id)
}
