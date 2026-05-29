package config

import "os"

type Config struct {
	Port         string
	DBHost       string
	DBUser       string
	DBPassword   string
	DBName       string
	KafkaBrokers string
	RedisAddr    string
}

func Load() *Config {
	return &Config{
		Port:         getEnv("APP_PORT", "8080"),
		DBHost:       getEnv("DB_HOST", "postgres:5432"),
		DBUser:       getEnv("DB_USER", "dev"),
		DBPassword:   getEnv("DB_PASSWORD", "dev"),
		DBName:       getEnv("DB_NAME", "tea_platform"),
		KafkaBrokers: getEnv("KAFKA_BROKERS", "kafka:9092"),
		RedisAddr:    getEnv("REDIS_ADDR", "redis:6379"),
	}
}

func getEnv(key, fallback string) string {
	if value, ok := os.LookupEnv(key); ok {
		return value
	}
	return fallback
}
