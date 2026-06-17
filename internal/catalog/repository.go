package catalog

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Repository — доступ к товарам/блюдам в PostgreSQL.
type Repository struct {
	pool *pgxpool.Pool
}

// NewRepository создаёт репозиторий поверх пула pgx.
func NewRepository(pool *pgxpool.Pool) *Repository {
	return &Repository{pool: pool}
}

const selectColumns = `id, kind, name, description, category, price_cents, currency, available, image_url, COALESCE(sku, ''), COALESCE(unit, '')`

// List возвращает позиции с опциональной фильтрацией по виду и категории.
// Пустые kind/category означают «без фильтра».
func (r *Repository) List(ctx context.Context, kind, category string) ([]Product, error) {
	const query = `
		SELECT ` + selectColumns + `
		FROM products
		WHERE ($1 = '' OR kind = $1)
		  AND ($2 = '' OR category = $2)
		ORDER BY name`

	rows, err := r.pool.Query(ctx, query, kind, category)
	if err != nil {
		return nil, fmt.Errorf("catalog: запрос списка: %w", err)
	}
	defer rows.Close()

	var products []Product
	for rows.Next() {
		p, err := scanProduct(rows)
		if err != nil {
			return nil, err
		}
		products = append(products, p)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("catalog: чтение строк: %w", err)
	}
	return products, nil
}

// Get возвращает позицию по идентификатору или ErrNotFound.
func (r *Repository) Get(ctx context.Context, id string) (Product, error) {
	const query = `SELECT ` + selectColumns + ` FROM products WHERE id = $1`

	p, err := scanProduct(r.pool.QueryRow(ctx, query, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return Product{}, ErrNotFound
	}
	if err != nil {
		return Product{}, fmt.Errorf("catalog: запрос по id: %w", err)
	}
	return p, nil
}

// Update меняет цену и доступность позиции, возвращает обновлённую запись.
func (r *Repository) Update(ctx context.Context, id string, priceCents int64, available bool) (Product, error) {
	const query = `
		UPDATE products SET price_cents = $2, available = $3
		WHERE id = $1
		RETURNING ` + selectColumns

	p, err := scanProduct(r.pool.QueryRow(ctx, query, id, priceCents, available))
	if errors.Is(err, pgx.ErrNoRows) {
		return Product{}, ErrNotFound
	}
	if err != nil {
		return Product{}, fmt.Errorf("catalog: обновление: %w", err)
	}
	return p, nil
}

// scanRow абстрагирует Scan у pgx.Row и pgx.Rows.
type scanRow interface {
	Scan(dest ...any) error
}

func scanProduct(row scanRow) (Product, error) {
	var p Product
	err := row.Scan(
		&p.ID, &p.Kind, &p.Name, &p.Description, &p.Category,
		&p.PriceCents, &p.Currency, &p.Available, &p.ImageURL, &p.SKU, &p.Unit,
	)
	if err != nil {
		return Product{}, err
	}
	return p, nil
}
