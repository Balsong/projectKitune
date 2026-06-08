-- Каталог: единая таблица товаров магазина (чай) и блюд ресторана.
-- kind различает домены, category — подкатегорию внутри домена.

CREATE TABLE IF NOT EXISTS products (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    kind        TEXT        NOT NULL CHECK (kind IN ('tea_goods', 'dish')),
    name        TEXT        NOT NULL,
    description TEXT        NOT NULL DEFAULT '',
    category    TEXT        NOT NULL DEFAULT '',
    price_cents BIGINT      NOT NULL CHECK (price_cents >= 0), -- цена в копейках
    currency    TEXT        NOT NULL DEFAULT 'RUB',
    available   BOOLEAN     NOT NULL DEFAULT TRUE,
    image_url   TEXT        NOT NULL DEFAULT '',
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (kind, name)
);

-- Частые выборки: по типу и по категории.
CREATE INDEX IF NOT EXISTS idx_products_kind ON products (kind);
CREATE INDEX IF NOT EXISTS idx_products_category ON products (category);

-- Сид-данные для MVP (цены в копейках).
INSERT INTO products (kind, name, description, category, price_cents, currency, available)
VALUES
    ('tea_goods', 'Да Хун Пао', 'Знаменитый уишаньский улун, насыщенный минеральный вкус', 'oolong', 85000, 'RUB', TRUE),
    ('tea_goods', 'Шу Пуэр «Лао Ча Тоу»', 'Выдержанный чай-смолка, мягкий землистый вкус', 'shu_puer', 62000, 'RUB', TRUE),
    ('dish',      'Дим-самы с креветками', 'Хар Гау на пару, 4 шт.', 'dim_sum', 42000, 'RUB', TRUE),
    ('dish',      'Утка по-пекински', 'Половина утки с блинчиками и соусом хойсин', 'main', 189000, 'RUB', TRUE)
ON CONFLICT (kind, name) DO NOTHING;
