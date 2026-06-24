package db

import (
	"context"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
)

// EnsureDatabase создаёт базу name, если её ещё нет. Подключается по
// adminDSN (к существующей БД того же кластера) и выполняет CREATE DATABASE.
// Нужен для самостоятельных сервисов со своей БД (напр. shop-svc → tea_shop),
// чтобы не зависеть от init-скриптов Postgres, выполняемых лишь на свежем томе.
func EnsureDatabase(ctx context.Context, adminDSN, name string) error {
	// Имя БД нельзя параметризовать в CREATE DATABASE — валидируем явно.
	if !validDBName(name) {
		return fmt.Errorf("db: недопустимое имя базы %q", name)
	}

	conn, err := pgx.Connect(ctx, adminDSN)
	if err != nil {
		return fmt.Errorf("db: подключение для создания БД: %w", err)
	}
	defer func() { _ = conn.Close(ctx) }()

	var exists bool
	if err := conn.QueryRow(ctx,
		`SELECT EXISTS(SELECT 1 FROM pg_database WHERE datname = $1)`, name,
	).Scan(&exists); err != nil {
		return fmt.Errorf("db: проверка существования БД: %w", err)
	}
	if exists {
		return nil
	}
	if _, err := conn.Exec(ctx, `CREATE DATABASE "`+name+`"`); err != nil {
		// Гонка двух стартующих сервисов: считаем «уже существует» успехом.
		if strings.Contains(err.Error(), "already exists") {
			return nil
		}
		return fmt.Errorf("db: создание БД %s: %w", name, err)
	}
	return nil
}

// validDBName допускает только безопасные идентификаторы (буквы, цифры, _).
func validDBName(name string) bool {
	if name == "" {
		return false
	}
	for _, r := range name {
		if !(r == '_' || (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9')) {
			return false
		}
	}
	return true
}
