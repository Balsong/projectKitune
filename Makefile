.PHONY: up down logs rebuild test

up:
	docker-compose up -d --build

down:
	docker-compose down -v

logs:
	docker-compose logs -f api-gateway

rebuild:
	docker-compose up -d --build --force-recreate

test:
	go test ./...

run-local:
	go run cmd/api-gateway/main.go