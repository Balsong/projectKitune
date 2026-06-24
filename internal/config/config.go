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
	MetricsPort  string // порт HTTP /metrics (все сервисы)
	GRPCPort     string // порт gRPC-сервера (catalog-svc и др.)
	CatalogAddr  string // адрес gRPC Catalog для клиентов (api-gateway, cart-svc)
	CartAddr     string // адрес gRPC Cart для клиентов (api-gateway, order-svc)
	OrderAddr    string // адрес gRPC Order для клиентов (api-gateway, payment, delivery)
	DeliveryAddr string // адрес gRPC Delivery для клиентов (api-gateway)
	AccountAddr  string // адрес gRPC Account для клиентов (api-gateway)
	BookingAddr  string // адрес gRPC Booking для клиентов (api-gateway)
	ShopAddr     string // адрес gRPC Shop для клиентов (api-gateway)
	DBHost       string
	DBPort       string
	DBUser       string
	DBPassword   string
	DBName       string
	KafkaBrokers string
	RedisAddr    string
	AdminEmails  string // список e-mail админов через запятую (бутстрап роли)

	// ShopDBName — имя отдельной БД интернет-магазина (свой контекст).
	ShopDBName string
	// ShopShipFlatCents — единый тариф доставки магазина по стране (копейки).
	ShopShipFlatCents int64
	// ShopShipFreeThresholdCents — порог бесплатной доставки (копейки).
	ShopShipFreeThresholdCents int64

	// PaymentLimitCents — порог mock-оплаты: заказы дороже отклоняются
	// («превышен лимит карты»). Позволяет демонстрировать компенсацию саги.
	PaymentLimitCents int64
}

// Load читает конфигурацию из окружения с разумными дефолтами.
func Load() *Config {
	return &Config{
		Port:         getEnv("APP_PORT", "8080"),
		MetricsPort:  getEnv("METRICS_PORT", "2112"),
		GRPCPort:     getEnv("GRPC_PORT", "9090"),
		CatalogAddr:  getEnv("CATALOG_ADDR", "catalog-svc:9090"),
		CartAddr:     getEnv("CART_ADDR", "cart-svc:9091"),
		OrderAddr:    getEnv("ORDER_ADDR", "order-svc:9092"),
		DeliveryAddr: getEnv("DELIVERY_ADDR", "delivery-svc:9093"),
		AccountAddr:  getEnv("ACCOUNT_ADDR", "account-svc:9094"),
		BookingAddr:  getEnv("BOOKING_ADDR", "booking-svc:9095"),
		ShopAddr:     getEnv("SHOP_ADDR", "shop-svc:9096"),
		DBHost:       getEnv("DB_HOST", "postgres"),
		DBPort:       getEnv("DB_PORT", "5432"),
		DBUser:       getEnv("DB_USER", "dev"),
		DBPassword:   getEnv("DB_PASSWORD", "dev"),
		DBName:       getEnv("DB_NAME", "tea_platform"),
		KafkaBrokers: getEnv("KAFKA_BROKERS", "kafka:9092"),
		RedisAddr:    getEnv("REDIS_ADDR", "redis:6379"),
		AdminEmails:  getEnv("ADMIN_EMAILS", ""),

		ShopDBName:                 getEnv("SHOP_DB_NAME", "tea_shop"),
		ShopShipFlatCents:          getEnvInt64("SHOP_SHIP_FLAT_CENTS", 35_000),  // 350 ₽
		ShopShipFreeThresholdCents: getEnvInt64("SHOP_SHIP_FREE_CENTS", 300_000), // 3000 ₽

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

// DSN формирует строку подключения к основной БД (tea_platform) для pgx.
func (c *Config) DSN() string {
	return c.dsnFor(c.DBName)
}

// ShopDSN — строка подключения к отдельной БД магазина (tea_shop).
func (c *Config) ShopDSN() string {
	return c.dsnFor(c.ShopDBName)
}

// MaintenanceDSN — подключение к основной БД для административных операций
// (например, CREATE DATABASE для tea_shop при первом старте shop-svc).
func (c *Config) MaintenanceDSN() string {
	return c.dsnFor(c.DBName)
}

func (c *Config) dsnFor(dbName string) string {
	return fmt.Sprintf(
		"postgres://%s:%s@%s:%s/%s?sslmode=disable",
		c.DBUser, c.DBPassword, c.DBHost, c.DBPort, dbName,
	)
}

func getEnv(key, fallback string) string {
	if value, ok := os.LookupEnv(key); ok {
		return value
	}
	return fallback
}
