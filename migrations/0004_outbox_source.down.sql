DROP INDEX IF EXISTS idx_outbox_unpublished;
CREATE INDEX IF NOT EXISTS idx_outbox_unpublished ON outbox (id) WHERE published_at IS NULL;
ALTER TABLE outbox DROP COLUMN IF EXISTS source;
