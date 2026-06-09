package config

import (
	"fmt"
	"os"
	"strconv"
)

// Config — конфигурация сервисов платформы, загружаемая из переменных
// окружения. Один тип на все сервисы; конкретный сервис использует нужные поля.
type Config struct {
	Port         string // HTTP-порт (api-gateway)
	GRPCPort     string // порт gRPC-сервера (catalog-svc и др.)
	CatalogAddr  string // адрес gRPC Catalog для клиентов (api-gateway, cart-svc)
	CartAddr     string // адрес gRPC Cart для клиентов (api-gateway, order-svc)
	OrderAddr    string // адрес gRPC Order для клиентов (api-gateway)
	DBHost       string
	DBPort       string
	DBUser       string
	DBPassword   string
	DBName       string
	KafkaBrokers string
	RedisAddr    string

	// PaymentLimitCents — порог mock-оплаты: заказы дороже отклоняются
	// («превышен лимит карты»). Позволяет демонстрировать компенсацию саги.
	PaymentLimitCents int64
}

// Load читает конфигурацию из окружения с разумными дефолтами.
func Load() *Config {
	return &Config{
		Port:         getEnv("APP_PORT", "8080"),
		GRPCPort:     getEnv("GRPC_PORT", "9090"),
		CatalogAddr:  getEnv("CATALOG_ADDR", "catalog-svc:9090"),
		CartAddr:     getEnv("CART_ADDR", "cart-svc:9091"),
		OrderAddr:    getEnv("ORDER_ADDR", "order-svc:9092"),
		DBHost:       getEnv("DB_HOST", "postgres"),
		DBPort:       getEnv("DB_PORT", "5432"),
		DBUser:       getEnv("DB_USER", "dev"),
		DBPassword:   getEnv("DB_PASSWORD", "dev"),
		DBName:       getEnv("DB_NAME", "tea_platform"),
		KafkaBrokers: getEnv("KAFKA_BROKERS", "kafka:9092"),
		RedisAddr:    getEnv("REDIS_ADDR", "redis:6379"),

		PaymentLimitCents: getEnvInt64("PAYMENT_LIMIT_CENTS", 1_000_000), // 10 000 ₽
	}
}

func getEnvInt64(key string, fallback int64) int64 {
	if value, ok := os.LookupEnv(key); ok {
		if n, err := strconv.ParseInt(value, 10, 64); err == nil {
			return n
		}
	}
	return fallback
}

// DSN формирует строку подключения к PostgreSQL для pgx.
func (c *Config) DSN() string {
	return fmt.Sprintf(
		"postgres://%s:%s@%s:%s/%s?sslmode=disable",
		c.DBUser, c.DBPassword, c.DBHost, c.DBPort, c.DBName,
	)
}

func getEnv(key, fallback string) string {
	if value, ok := os.LookupEnv(key); ok {
		return value
	}
	return fallback
}
