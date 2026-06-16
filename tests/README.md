# Тесты КИЦУНЭ

End-to-end проверка всех функций сайта через web-слой (nginx `:8081`) — тот же
путь, что видит браузер: раздача страниц/ассетов, `/api`-прокси и сквозные
бизнес-потоки (меню, корзина, заказ + сага, компенсация, бронь, статусы).

## Запуск

```bash
make up            # поднять стек (docker compose up -d)
tests/e2e.sh       # прогон против http://localhost:8081
```

Другой адрес:

```bash
BASE=http://localhost:8081 tests/e2e.sh
```

Код выхода `0` — все тесты прошли, `1` — есть провалы (печатается список).

## Что покрывается (`e2e.sh`)

| Раздел | Проверки |
|---|---|
| 0 · Доступность | web+gateway отвечают |
| 1 · Страницы | все 10 HTML отдаются (200) + SPA-fallback |
| 2 · Ассеты | css/kitsune.css, js/kitsune.js, js/catalog.js, assets/logo.svg |
| 3 · Меню | `/api/v1/menu`: 27 позиций, поля id/sku/name/price_cents/kind, чай+блюда |
| 4 · Корзина | add/повторный add/итог/remove/clear + валидация (несуществующий товар → 400) |
| 5 · Заказ + сага | пустая корзина → 409, неверный fulfillment → 400, checkout → сага `created→…→completed`, доставка `delivered`, очистка корзины |
| 6 · Компенсация | заказ дороже лимита оплаты → `payment_failed` (release резерва) |
| 7 · Бронь | `/api/v1/book` → `booking_id`, статус `booking_pending` |

## Требования

- Запущенный стек (`make up`). Для теста саги нужны order/inventory/payment/
  delivery/notification-svc — они входят в `docker compose up`.
- `python3` и `curl` (есть в системе по умолчанию на macOS/Linux).
- Совет: при слабых ресурсах останови тяжёлый ELK, чтобы консьюмеры Kafka не
  лагали: `docker compose stop elasticsearch kibana filebeat`.
