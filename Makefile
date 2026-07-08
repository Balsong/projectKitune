.PHONY: up down logs rebuild test run-local web test-e2e test-browser

# ── Полный стек ──
# После --build старый образ пересобранного сервиса остаётся dangling (<none>) —
# подчищаем его сразу, чтобы диск Docker VM не забивался от пересборки к пересборке.
up:
	docker compose up -d --build
	docker image prune -f --filter "dangling=true"

down:
	docker compose down -v

logs:
	docker compose logs -f api-gateway

rebuild:
	docker compose up -d --build --force-recreate
	docker image prune -f --filter "dangling=true"

test:
	go test ./...

run-local:
	go run cmd/api-gateway/main.go

# ── Frontend (web/) — статический сайт КИЦУНЭ ──
# Пересобрать и поднять только фронтенд (nginx раздаёт статику, /api → gateway).
# Сайт: http://localhost:8081
web:
	docker compose up -d --build web
	docker image prune -f --filter "dangling=true"

# ── E2E-тесты сайта (через web :8081) ──
test-e2e:
	bash tests/e2e.sh

# Браузерные (Playwright) тесты. Требуют один раз: cd tests/browser && npm install && npm run install:browsers
test-browser:
	cd tests/browser && npm test
