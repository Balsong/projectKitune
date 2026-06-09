-- Инвентарь: остатки магазина (чай) и стоп-лист ресторана (блюда).
-- available_qty IS NULL означает «неограниченно» (блюда готовятся по заказу,
-- ограничены только стоп-листом).

CREATE TABLE IF NOT EXISTS inventory (
    product_id   UUID PRIMARY KEY,
    kind         TEXT    NOT NULL,
    available_qty BIGINT,                      -- NULL = неограниченно (блюда)
    in_stoplist  BOOLEAN NOT NULL DEFAULT FALSE,
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Резервы под заказы. UNIQUE(order_id, product_id) обеспечивает идемпотентность
-- при повторной доставке order.created.
CREATE TABLE IF NOT EXISTS reservations (
    id         BIGSERIAL PRIMARY KEY,
    order_id   UUID    NOT NULL,
    product_id UUID    NOT NULL,
    quantity   INT     NOT NULL CHECK (quantity > 0),
    status     TEXT    NOT NULL DEFAULT 'reserved', -- reserved | committed | released
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (order_id, product_id)
);

CREATE INDEX IF NOT EXISTS idx_reservations_order ON reservations (order_id);

-- Сид остатков из каталога: чай — 100 шт на складе, блюда — без лимита.
INSERT INTO inventory (product_id, kind, available_qty, in_stoplist)
SELECT id, kind, CASE WHEN kind = 'tea_goods' THEN 100 ELSE NULL END, FALSE
FROM products
ON CONFLICT (product_id) DO NOTHING;
