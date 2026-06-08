package config

import (
	"fmt"
	"os"
)

// Config — конфигурация сервисов платформы, загружаемая из переменных
// окружения. Один тип на все сервисы; конкретный сервис использует нужные поля.
type Config struct {
	Port         string // HTTP-порт (api-gateway)
	GRPCPort     string // порт gRPC-сервера (catalog-svc и др.)
	CatalogAddr  string // адрес gRPC Catalog для клиентов (api-gateway)
	DBHost       string
	DBPort       string
	DBUser       string
	DBPassword   string
	DBName       string
	KafkaBrokers string
	RedisAddr    string
}

// Load читает конфигурацию из окружения с разумными дефолтами.
func Load() *Config {
	return &Config{
		Port:         getEnv("APP_PORT", "8080"),
		GRPCPort:     getEnv("GRPC_PORT", "9090"),
		CatalogAddr:  getEnv("CATALOG_ADDR", "catalog-svc:9090"),
		DBHost:       getEnv("DB_HOST", "postgres"),
		DBPort:       getEnv("DB_PORT", "5432"),
		DBUser:       getEnv("DB_USER", "dev"),
		DBPassword:   getEnv("DB_PASSWORD", "dev"),
		DBName:       getEnv("DB_NAME", "tea_platform"),
		KafkaBrokers: getEnv("KAFKA_BROKERS", "kafka:9092"),
		RedisAddr:    getEnv("REDIS_ADDR", "redis:6379"),
	}
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
