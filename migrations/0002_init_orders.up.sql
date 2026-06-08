-- Заказы, их позиции и transactional outbox.

CREATE TABLE IF NOT EXISTS orders (
    id               UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id          TEXT        NOT NULL DEFAULT '',
    cart_id          TEXT        NOT NULL DEFAULT '',
    fulfillment_type TEXT        NOT NULL, -- shop_delivery | food_courier | dine_in
    status           TEXT        NOT NULL, -- created | payment_pending | paid | ...
    total_cents      BIGINT      NOT NULL CHECK (total_cents >= 0),
    currency         TEXT        NOT NULL DEFAULT 'RUB',
    address          TEXT        NOT NULL DEFAULT '',
    created_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at       TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_orders_user ON orders (user_id);
CREATE INDEX IF NOT EXISTS idx_orders_status ON orders (status);

CREATE TABLE IF NOT EXISTS order_items (
    id               BIGSERIAL PRIMARY KEY,
    order_id         UUID   NOT NULL REFERENCES orders (id) ON DELETE CASCADE,
    product_id       TEXT   NOT NULL,
    name             TEXT   NOT NULL,
    quantity         INT    NOT NULL CHECK (quantity > 0),
    unit_price_cents BIGINT NOT NULL,
    subtotal_cents   BIGINT NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_order_items_order ON order_items (order_id);

-- Transactional outbox: событие пишется в одной транзакции с заказом,
-- relay-воркер асинхронно публикует его в Kafka и помечает отправленным.
CREATE TABLE IF NOT EXISTS outbox (
    id           BIGSERIAL PRIMARY KEY,
    aggregate_id TEXT        NOT NULL,
    topic        TEXT        NOT NULL,
    event_type   TEXT        NOT NULL,
    payload      BYTEA       NOT NULL, -- сериализованный конверт events.Envelope
    attempts     INT         NOT NULL DEFAULT 0,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    published_at TIMESTAMPTZ
);

-- Частичный индекс по неопубликованным — relay выбирает только их.
CREATE INDEX IF NOT EXISTS idx_outbox_unpublished ON outbox (id) WHERE published_at IS NULL;
