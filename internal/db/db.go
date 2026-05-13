package db

import (
	"context"
	"fmt"
	"log"
)

type DB struct {
	// TODO: сюда подключим *pgxpool.Pool или *sql.DB
}

func NewDB(ctx context.Context, host, user, pass, dbname string) (*DB, error) {
	// Пока просто логируем. Реальное подключение добавим на следующем этапе.
	connStr := fmt.Sprintf("postgresql://%s:%s@%s/%s", user, pass, host, dbname)
	log.Printf("🔌 [DB] Подготовка подключения к: %s", connStr)

	return &DB{}, nil
}
