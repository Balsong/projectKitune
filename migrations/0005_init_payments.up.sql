-- Платежи. order_id как PK обеспечивает идемпотентность: один заказ — один платёж.

CREATE TABLE IF NOT EXISTS payments (
    order_id     UUID PRIMARY KEY,
    amount_cents BIGINT      NOT NULL,
    currency     TEXT        NOT NULL DEFAULT 'RUB',
    status       TEXT        NOT NULL, -- succeeded | failed
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);
