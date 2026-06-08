# Универсальный multi-stage Dockerfile: какой сервис собирать — задаётся
# build-аргументом SERVICE (имя каталога в ./cmd). По умолчанию api-gateway.
FROM golang:1.25-alpine AS builder

WORKDIR /app

# go.su[m] — опциональный паттерн: подхватит go.sum, если он есть.
COPY go.mod go.su[m] ./
RUN go mod download

COPY . .

ARG SERVICE=api-gateway
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /app/bin/service ./cmd/${SERVICE}

FROM alpine:latest

RUN apk --no-cache add ca-certificates tzdata

WORKDIR /root/

COPY --from=builder /app/bin/service .

CMD ["./service"]
