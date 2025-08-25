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
	PodID         string
}

// Load загружает конфигурацию из переменных окружения
func Load() *Config {
	return &Config{
		ServerAddr:    getEnv("SERVER_ADDR", ":8080"),
		RedisAddr:     getEnv("REDIS_ADDR", "localhost:6379"),
		RedisPassword: getEnv("REDIS_PASSWORD", ""),
		RedisDB:       0, // Всегда используем DB 0 для простоты
		PodID:         defaultPodID(),
	}
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func defaultPodID() string {
	if v := os.Getenv("POD_ID"); v != "" {
		return v
	}
	if h, err := os.Hostname(); err == nil && h != "" {
		return h
	}
	return "pod-unknown"
}
