package db

import (
	"fmt"
	"log"
)

// DB — обёртка над пулом подключений к Postgres.
// На следующем этапе сюда подключим *pgxpool.Pool.
type DB struct {
	// TODO(Этап 1): заменить заглушку на реальный *pgxpool.Pool
}

// NewDB подготавливает подключение к базе данных.
// Пока соединение не открывается реально — это произойдёт на этапе
// подключения pgx. Сигнатура согласована с вызовом в api-gateway.
func NewDB(host, user, pass, dbname string) (*DB, error) {
	connStr := fmt.Sprintf("postgresql://%s:%s@%s/%s", user, pass, host, dbname)
	log.Printf("🔌 [DB] Подготовка подключения к: %s", connStr)

	return &DB{}, nil
}

// Close освобождает ресурсы пула подключений.
func (d *DB) Close() error {
	log.Printf("🔌 [DB] Закрытие подключения")
	return nil
}
