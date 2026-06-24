-- Схема интернет-магазина чая (БД tea_shop). Отдельный контекст: товары с
-- реальным складским остатком, изолированная корзина, заказы с доставкой по стране.

CREATE TABLE IF NOT EXISTS products (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    sku          TEXT        NOT NULL UNIQUE,
    name         TEXT        NOT NULL,
    description  TEXT        NOT NULL DEFAULT '',
    category     TEXT        NOT NULL, -- puer | oolong | red | white | green | teaware | gift
    price_cents  BIGINT      NOT NULL CHECK (price_cents >= 0),
    currency     TEXT        NOT NULL DEFAULT 'RUB',
    weight_grams INT         NOT NULL DEFAULT 0,
    unit         TEXT        NOT NULL DEFAULT '', -- «50 г», «357 г (блин)», «1 шт»
    image_url    TEXT        NOT NULL DEFAULT '',
    stock_qty    INT         NOT NULL DEFAULT 0 CHECK (stock_qty >= 0),
    available    BOOLEAN     NOT NULL DEFAULT TRUE,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_shop_products_category ON products (category);

-- Корзина магазина (в БД, чтобы остаток и резерв считались транзакционно).
CREATE TABLE IF NOT EXISTS cart_items (
    cart_id    TEXT        NOT NULL,
    product_id UUID        NOT NULL REFERENCES products (id) ON DELETE CASCADE,
    quantity   INT         NOT NULL CHECK (quantity > 0),
    added_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (cart_id, product_id)
);

CREATE TABLE IF NOT EXISTS orders (
    id                   UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id              TEXT        NOT NULL DEFAULT '',
    status               TEXT        NOT NULL DEFAULT 'created', -- created|paid|shipped|delivered|cancelled
    customer             TEXT        NOT NULL DEFAULT '',
    phone                TEXT        NOT NULL DEFAULT '',
    email                TEXT        NOT NULL DEFAULT '',
    country              TEXT        NOT NULL DEFAULT 'Россия',
    region               TEXT        NOT NULL DEFAULT '',
    city                 TEXT        NOT NULL DEFAULT '',
    address              TEXT        NOT NULL DEFAULT '',
    postal_code          TEXT        NOT NULL DEFAULT '',
    comment              TEXT        NOT NULL DEFAULT '',
    items_subtotal_cents BIGINT      NOT NULL CHECK (items_subtotal_cents >= 0),
    delivery_cents       BIGINT      NOT NULL DEFAULT 0,
    total_cents          BIGINT      NOT NULL CHECK (total_cents >= 0),
    tracking_code        TEXT        NOT NULL DEFAULT '',
    created_at           TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at           TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_shop_orders_user ON orders (user_id);
CREATE INDEX IF NOT EXISTS idx_shop_orders_status ON orders (status);

CREATE TABLE IF NOT EXISTS order_items (
    id               BIGSERIAL PRIMARY KEY,
    order_id         UUID   NOT NULL REFERENCES orders (id) ON DELETE CASCADE,
    product_id       TEXT   NOT NULL,
    name             TEXT   NOT NULL,
    quantity         INT    NOT NULL CHECK (quantity > 0),
    unit_price_cents BIGINT NOT NULL,
    subtotal_cents   BIGINT NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_shop_order_items_order ON order_items (order_id);

-- Transactional outbox магазина (свой, в БД tea_shop). Совместим с
-- internal/outbox: колонки source/aggregate_id/topic/event_type/payload.
CREATE TABLE IF NOT EXISTS outbox (
    id           BIGSERIAL PRIMARY KEY,
    source       TEXT        NOT NULL DEFAULT 'shop-svc',
    aggregate_id TEXT        NOT NULL,
    topic        TEXT        NOT NULL,
    event_type   TEXT        NOT NULL,
    payload      BYTEA       NOT NULL,
    attempts     INT         NOT NULL DEFAULT 0,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    published_at TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_shop_outbox_unpublished ON outbox (id) WHERE published_at IS NULL;
