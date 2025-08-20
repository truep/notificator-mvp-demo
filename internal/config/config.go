package config

import (
	"os"
)

// Config содержит конфигурацию приложения
type Config struct {
	ServerAddr    string
	RedisAddr     string
	RedisPassword string
	RedisDB       int
}

// Load загружает конфигурацию из переменных окружения
func Load() *Config {
	return &Config{
		ServerAddr:    getEnv("SERVER_ADDR", ":8080"),
		RedisAddr:     getEnv("REDIS_ADDR", "localhost:6379"),
		RedisPassword: getEnv("REDIS_PASSWORD", ""),
		RedisDB:       0, // Всегда используем DB 0 для простоты
	}
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}
