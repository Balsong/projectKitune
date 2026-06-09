-- Доставки. order_id как PK — одна доставка на заказ (идемпотентность).

CREATE TABLE IF NOT EXISTS deliveries (
    order_id         UUID PRIMARY KEY,
    fulfillment_type TEXT        NOT NULL,
    status           TEXT        NOT NULL, -- created | dispatched | delivered
    tracking_code    TEXT        NOT NULL DEFAULT '',
    courier          TEXT        NOT NULL DEFAULT '',
    eta              TEXT        NOT NULL DEFAULT '',
    correlation_id   TEXT        NOT NULL DEFAULT '', -- сквозной id саги
    created_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at       TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_deliveries_status ON deliveries (status);

-- Привязка заказа к брони стола (для dine_in).
ALTER TABLE orders ADD COLUMN IF NOT EXISTS booking_id TEXT NOT NULL DEFAULT '';
