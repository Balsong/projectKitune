.PHONY: up down logs rebuild test run-local web

# ── Полный стек ──
up:
	docker compose up -d --build

down:
	docker compose down -v

logs:
	docker compose logs -f api-gateway

rebuild:
	docker compose up -d --build --force-recreate

test:
	go test ./...

run-local:
	go run cmd/api-gateway/main.go

# ── Frontend (web/) — статический сайт КИЦУНЭ ──
# Пересобрать и поднять только фронтенд (nginx раздаёт статику, /api → gateway).
# Сайт: http://localhost:8081
web:
	docker compose up -d --build web
