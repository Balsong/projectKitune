-- Брони столов. user_id пустой для гостевых броней.

CREATE TABLE IF NOT EXISTS bookings (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id    TEXT        NOT NULL DEFAULT '',
    customer   TEXT        NOT NULL DEFAULT '',
    phone      TEXT        NOT NULL DEFAULT '',
    guests     INT         NOT NULL DEFAULT 0,
    time_slot  TEXT        NOT NULL DEFAULT '', -- дата и время визита (текст)
    comment    TEXT        NOT NULL DEFAULT '',
    status     TEXT        NOT NULL DEFAULT 'new', -- new | confirmed | cancelled
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_bookings_user ON bookings (user_id);
CREATE INDEX IF NOT EXISTS idx_bookings_status ON bookings (status);
