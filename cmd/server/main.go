package main

import (
	"context"
	"log"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"notification-mvp/internal/config"
	"notification-mvp/internal/handler"
	"notification-mvp/internal/repository"
	"notification-mvp/internal/service"
	"notification-mvp/internal/websocket"
	"notification-mvp/internal/worker"

	"github.com/redis/go-redis/v9"
)

func main() {
	// Настраиваем логгер
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))
	slog.SetDefault(logger)

	// Загружаем конфигурацию
	cfg := config.Load()

	ctx := context.Background()

	// Инициализируем Redis клиент
	rdb := redis.NewClient(&redis.Options{
		Addr:     cfg.RedisAddr,
		Password: cfg.RedisPassword,
		DB:       cfg.RedisDB,
	})

	// Проверяем подключение к Redis
	if err := rdb.Ping(ctx).Err(); err != nil {
		log.Fatalf("Не удалось подключиться к Redis: %v", err)
	}

	slog.Info("Подключились к Redis", "addr", cfg.RedisAddr)

	// Инициализируем слои
	repo := repository.NewRedisRepository(rdb)
	notifyService := service.NewNotificationService(repo, logger)
	connectionManager := websocket.NewConnectionManager(logger)
	handlers := handler.NewHandlers(notifyService, repo, connectionManager, logger)

	// Создаем HTTP сервер
	mux := http.NewServeMux()

	// Регистрируем маршруты
	mux.HandleFunc("POST /api/v1/notify", handlers.NotifyHandler)
	mux.HandleFunc("GET /ws", handlers.WebSocketHandler)
	mux.HandleFunc("GET /health", handlers.HealthHandler)

	// Админ API
	mux.HandleFunc("GET /api/v1/admin/clients", handlers.ConnectedClientsHandler)
	mux.HandleFunc("GET /api/v1/admin/pending", handlers.PendingNotificationsHandler)
	mux.HandleFunc("GET /api/v1/admin/users", handlers.AvailableUsersHandler)
	mux.HandleFunc("GET /api/v1/admin/history", handlers.HistoryHandler)

	mux.HandleFunc("GET /", handlers.IndexHandler) // Для тестового клиента

	// Запускаем фоновые воркеры
	ttlJanitor := worker.NewTTLJanitor(repo, logger)
	groupMaintenance := worker.NewGroupMaintenance(repo, logger)

	go ttlJanitor.Start(ctx)
	go groupMaintenance.Start(ctx)

	slog.Info("Запущены фоновые воркеры")

	// Создаем HTTP сервер
	server := &http.Server{
		Addr:    cfg.ServerAddr,
		Handler: mux,
	}

	// Запускаем сервер в горутине
	go func() {
		slog.Info("Запуск HTTP сервера", "addr", cfg.ServerAddr)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Ошибка запуска сервера: %v", err)
		}
	}()

	// Ожидаем сигнал завершения
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	slog.Info("Завершение работы...")

	// Graceful shutdown
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := server.Shutdown(ctx); err != nil {
		slog.Error("Ошибка при завершении сервера", "error", err)
	}

	if err := rdb.Close(); err != nil {
		slog.Error("Ошибка при закрытии Redis", "error", err)
	}

	slog.Info("Сервер завершен")
}
