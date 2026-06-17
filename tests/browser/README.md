# Браузерные тесты (Playwright)

Кликовые end-to-end сценарии поверх живого сайта: навигация по страницам,
корзина, регистрация/смена пароля, проверка SEO (canonical/OG/robots/sitemap).
Дополняют API-тесты `tests/e2e.sh`.

## Предварительно

Поднять стек (сайт на `http://localhost:8081`):

```bash
docker compose up -d
```

## Установка

```bash
cd tests/browser
npm install
npm run install:browsers   # скачивает Chromium для Playwright
```

## Запуск

```bash
npm test                       # против http://localhost:8081
BASE_URL=http://localhost:8081 npm test
npm run test:headed            # с видимым браузером
npm run report                 # HTML-отчёт последнего прогона
```

## Что покрыто

- `specs/smoke.spec.js` — открытие всех публичных страниц, шапка/подвал,
  переходы по меню, SEO (canonical, og:title, description, robots.txt, sitemap.xml).
- `specs/cart.spec.js` — добавление позиции из меню → счётчик корзины и страница корзины.
- `specs/auth.spec.js` — регистрация через форму → кабинет; смена пароля; ошибка входа.
