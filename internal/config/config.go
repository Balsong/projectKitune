package config

import "os"

type Config struct {
	Port         string
	DBHost       string
	DBUser       string
	DBPassword   string
	DBName       string
	KafkaBrokers string
}

func Load() *Config {
	return &Config{
		Port:         getEnv("APP_PORT", "8080"),
		DBHost:       getEnv("DB_HOST", "localhost:5432"),
		DBUser:       getEnv("DB_USER", "postgres"),
		DBPassword:   getEnv("DB_PASSWORD", "dev"),
		DBName:       getEnv("DB_NAME", "tea_platform"),
		KafkaBrokers: getEnv("KAFKA_BROKERS", "localhost:9092"),
	}
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
