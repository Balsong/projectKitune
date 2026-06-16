-- Реальное меню «Дом чая КИЦУНЭ» (по чайной карте заведения).
-- Категории: chinese_tea | coffee | cold | author_tea | boiled_tea.
-- Напитки готовятся под заказ → инвентарь без лимита (made-to-order).

ALTER TABLE products ADD COLUMN IF NOT EXISTS unit TEXT NOT NULL DEFAULT '';

DELETE FROM products;

INSERT INTO products (sku, kind, name, description, category, price_cents, currency, available, unit) VALUES
  -- Китайский чай (на 2 человек)
  ('cha-red',          'tea_goods', 'Красный чай',     'Классический китайский красный чай — мягкий, согревающий.',                 'chinese_tea', 52000, 'RUB', TRUE, '2 л · на 2 чел.'),
  ('cha-gaba',         'tea_goods', 'Габа',            'Улун ГАБА с лёгкой кислинкой и фруктовым тоном.',                            'chinese_tea', 60000, 'RUB', TRUE, '1 л'),
  ('cha-white',        'tea_goods', 'Белый чай',       'Нежный белый чай — деликатный, медовый, прозрачный.',                        'chinese_tea', 50000, 'RUB', TRUE, '1 л'),
  ('cha-green',        'tea_goods', 'Зелёный чай',     'Свежий зелёный чай с травянистой лёгкостью.',                                'chinese_tea', 50000, 'RUB', TRUE, '2 л'),
  ('cha-dark-oolong',  'tea_goods', 'Тёмный улун',     'Тёмный улун: печёные фрукты, минеральность, плотность.',                     'chinese_tea', 55000, 'RUB', TRUE, '2 л'),
  ('cha-light-oolong', 'tea_goods', 'Светлый улун',    'Светлый улун — цветочный, сливочный, многопроливной.',                       'chinese_tea', 54000, 'RUB', TRUE, '2 л'),
  ('cha-sheng',        'tea_goods', 'Шен Пуэр',        'Живой светлый пуэр: сухофрукты, мёд, бодрящая терпкость.',                   'chinese_tea', 50000, 'RUB', TRUE, '2 л'),
  ('cha-shu',          'tea_goods', 'Шу Пуэр',         'Плотный землистый пуэр с нотами какао и влажного дерева.',                   'chinese_tea', 50000, 'RUB', TRUE, '1 л'),

  -- Кофейные напитки
  ('cof-espresso',     'dish', 'Эспрессо',           'Классический эспрессо.',                                    'coffee', 15000, 'RUB', TRUE, '40 мл'),
  ('cof-americano',    'dish', 'Американо',          'Эспрессо с горячей водой.',                                 'coffee', 18000, 'RUB', TRUE, '0,2 / 0,3 л · 180/210 ₽'),
  ('cof-cappuccino',   'dish', 'Капучино',           'Эспрессо с молочной пеной.',                                'coffee', 19000, 'RUB', TRUE, '0,2 / 0,3 л · 190/230 ₽'),
  ('cof-latte',        'dish', 'Латте',              'Мягкий латте на эспрессо и молоке.',                        'coffee', 22000, 'RUB', TRUE, '0,3 л'),
  ('cof-raf',          'dish', 'Раф',                'Сливочный раф с ванилью.',                                  'coffee', 21000, 'RUB', TRUE, '0,2 / 0,3 л · 210/260 ₽'),
  ('cof-cocoa',        'dish', 'Какао',              'Горячее какао на молоке.',                                  'coffee', 18500, 'RUB', TRUE, '0,2 / 0,3 л · 185/205 ₽'),

  -- Холодные напитки (400 мл)
  ('cold-milkshake',   'dish', 'Молочный коктейль',  'Молоко, ванильное мороженое, сироп по желанию.',            'cold', 28000, 'RUB', TRUE, '400 мл'),
  ('cold-latte',       'dish', 'Холодный латте',     'Освежающий айс-латте.',                                     'cold', 27000, 'RUB', TRUE, '400 мл'),
  ('cold-americano',   'dish', 'Холодный американо', 'Айс-американо со льдом.',                                    'cold', 21000, 'RUB', TRUE, '400 мл'),
  ('cold-strawberry',  'dish', 'Клубничный мохито',  'Содовая, клубника, мята, лимон, лёд.',                      'cold', 25000, 'RUB', TRUE, '400 мл'),
  ('cold-mojito',      'dish', 'Мохито',             'Содовая, мята, лимон, лёд.',                                'cold', 24000, 'RUB', TRUE, '400 мл'),
  ('cold-lychee',      'dish', 'Личи Тайфун',        'Содовая, зелёный чай, сироп личи, лимон, апельсин, лёд.',   'cold', 25000, 'RUB', TRUE, '400 мл'),
  ('cold-seabuck',     'dish', 'Облепиховое Облако', 'Содовая, облепиха, апельсин, лёд.',                         'cold', 29000, 'RUB', TRUE, '400 мл'),
  ('cold-peach',       'dish', 'Персиковый Сапфир',  'Содовая, чёрный чай, анчан, сироп персика, лимон, лёд.',    'cold', 25000, 'RUB', TRUE, '400 мл'),
  ('cold-lavender',    'dish', 'Лавандовый',         'Содовая, лаванда, лавандовый сироп, лёд.',                  'cold', 24000, 'RUB', TRUE, '400 мл'),
  ('cold-ruby',        'dish', 'Шёлковый Рубин',     'Каркаде, лимонный фреш, сгущённое молоко, лёд.',            'cold', 29000, 'RUB', TRUE, '400 мл'),

  -- Авторский чай
  ('auth-masala',      'tea_goods', 'Масала',            'Крепкий чёрный чай, специи, молоко.',                          'author_tea', 58000, 'RUB', TRUE, '900 мл'),
  ('auth-dhp',         'tea_goods', 'Уютный ДХП',        'Да Хун Пао Премиум, корица, бадьян, вишнёвый сок.',            'author_tea', 95000, 'RUB', TRUE, '900 мл'),
  ('auth-herbal',      'tea_goods', 'Травяной/Фруктовый','Авторский травяной/фруктовый купаж.',                          'author_tea', 44000, 'RUB', TRUE, '900 мл / 0,3 л · 440/160 ₽'),
  ('auth-chinese',     'tea_goods', 'Китайский чай',     'Китайский чай в авторской подаче.',                            'author_tea', 20000, 'RUB', TRUE, '0,3 л'),
  ('auth-seabuck',     'tea_goods', 'Облепиховый',       'Китайский зелёный чай с облепихой, апельсином и лимоном.',     'author_tea', 60000, 'RUB', TRUE, '900 мл / 0,3 л · 600/250 ₽'),
  ('auth-cranberry',   'tea_goods', 'Клюквенный',        'Чёрный чай, клюква, апельсин.',                                'author_tea', 60000, 'RUB', TRUE, '900 мл / 0,3 л · 600/250 ₽'),
  ('auth-pear',        'tea_goods', 'Грушевый',          'Жасминовый чай высшей категории, пюре груши и банана, корица.','author_tea', 50000, 'RUB', TRUE, '900 мл / 0,3 л · 500/250 ₽'),

  -- Варёный чай «В чайном каноне Лу Юя» (900 мл)
  ('boil-shu-water',   'tea_goods', 'Шу Пуэр на воде',  'Варёный Шу Пуэр на воде — густой, сладкий.',     'boiled_tea', 75000, 'RUB', TRUE, '900 мл'),
  ('boil-shu-juice',   'tea_goods', 'Шу Пуэр на соке',  'Варёный Шу Пуэр на соке — ягодная глубина.',     'boiled_tea', 95000, 'RUB', TRUE, '900 мл'),
  ('boil-sheng-water', 'tea_goods', 'Шен Пуэр на воде', 'Варёный Шен Пуэр на воде — свежий, бодрящий.',   'boiled_tea', 75000, 'RUB', TRUE, '900 мл'),
  ('boil-sheng-juice', 'tea_goods', 'Шен Пуэр на соке', 'Варёный Шен Пуэр на соке.',                      'boiled_tea', 85000, 'RUB', TRUE, '900 мл'),
  ('boil-gaba-water',  'tea_goods', 'Габа на воде',     'Варёная ГАБА на воде.',                          'boiled_tea', 90000, 'RUB', TRUE, '900 мл');

CREATE UNIQUE INDEX IF NOT EXISTS idx_products_sku ON products (sku);

-- Инвентарь: всё под заказ, без лимита остатка.
DELETE FROM inventory;
INSERT INTO inventory (product_id, kind, available_qty, in_stoplist)
SELECT id, kind, NULL, FALSE FROM products
ON CONFLICT (product_id) DO NOTHING;
