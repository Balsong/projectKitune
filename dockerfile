FROM golang:1.24-alpine AS builder

WORKDIR /app

# go.su[m] — опциональный паттерн: подхватит go.sum, если он есть,
# и не упадёт, пока внешних зависимостей (а значит и go.sum) ещё нет.
COPY go.mod go.su[m] ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /app/bin/api-gateway ./cmd/api-gateway

FROM alpine:latest

RUN apk --no-cache add ca-certificates tzdata

WORKDIR /root/

COPY --from=builder /app/bin/api-gateway .

EXPOSE 8080

CMD ["./api-gateway"]