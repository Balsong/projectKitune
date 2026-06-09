-- Изоляция outbox по сервису-источнику: в общей БД несколько relay-воркеров,
-- каждый должен публиковать только свои события (иначе дубли).

ALTER TABLE outbox ADD COLUMN IF NOT EXISTS source TEXT NOT NULL DEFAULT '';

-- Индекс под выборку неопубликованных конкретного источника.
DROP INDEX IF EXISTS idx_outbox_unpublished;
CREATE INDEX IF NOT EXISTS idx_outbox_unpublished
    ON outbox (source, id) WHERE published_at IS NULL;
