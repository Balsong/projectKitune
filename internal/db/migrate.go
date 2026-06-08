package db

import (
	"context"
	"fmt"
	"io/fs"
	"sort"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Migrate применяет все *.up.sql миграции из fsys по возрастанию версии,
// пропуская уже применённые. Версия — число в начале имени файла
// (например, "0001_init_catalog.up.sql" → 1). Каждая миграция применяется
// в отдельной транзакции вместе с записью версии в schema_migrations.
func Migrate(ctx context.Context, pool *pgxpool.Pool, fsys fs.FS) error {
	const createTable = `
		CREATE TABLE IF NOT EXISTS schema_migrations (
			version    BIGINT PRIMARY KEY,
			applied_at TIMESTAMPTZ NOT NULL DEFAULT now()
		)`
	if _, err := pool.Exec(ctx, createTable); err != nil {
		return fmt.Errorf("db: создание schema_migrations: %w", err)
	}

	files, err := fs.Glob(fsys, "*.up.sql")
	if err != nil {
		return fmt.Errorf("db: поиск миграций: %w", err)
	}
	sort.Strings(files)

	for _, name := range files {
		version, err := parseVersion(name)
		if err != nil {
			return err
		}

		var applied bool
		if err := pool.QueryRow(ctx,
			`SELECT EXISTS(SELECT 1 FROM schema_migrations WHERE version = $1)`,
			version,
		).Scan(&applied); err != nil {
			return fmt.Errorf("db: проверка версии %d: %w", version, err)
		}
		if applied {
			continue
		}

		statements, err := fs.ReadFile(fsys, name)
		if err != nil {
			return fmt.Errorf("db: чтение %s: %w", name, err)
		}

		if err := applyMigration(ctx, pool, version, string(statements)); err != nil {
			return fmt.Errorf("db: применение %s: %w", name, err)
		}
	}
	return nil
}

// applyMigration выполняет SQL миграции и фиксирует её версию атомарно.
func applyMigration(ctx context.Context, pool *pgxpool.Pool, version int64, statements string) error {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }() // no-op после успешного Commit

	if _, err := tx.Exec(ctx, statements); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx,
		`INSERT INTO schema_migrations (version) VALUES ($1)`, version,
	); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// parseVersion извлекает числовой префикс имени файла миграции.
func parseVersion(filename string) (int64, error) {
	idx := strings.Index(filename, "_")
	if idx <= 0 {
		return 0, fmt.Errorf("db: не удалось определить версию миграции из %q", filename)
	}
	version, err := strconv.ParseInt(filename[:idx], 10, 64)
	if err != nil {
		return 0, fmt.Errorf("db: неверная версия в %q: %w", filename, err)
	}
	return version, nil
}
