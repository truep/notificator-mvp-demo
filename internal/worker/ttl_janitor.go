package worker

import (
	"context"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"time"

	"notification-mvp/internal/domain"
)

// TTLJanitor отвечает за удаление просроченных уведомлений
type TTLJanitor struct {
	repo   domain.NotificationRepository
	logger *slog.Logger
}

// NewTTLJanitor создает новый экземпляр TTLJanitor
func NewTTLJanitor(repo domain.NotificationRepository, logger *slog.Logger) *TTLJanitor {
	return &TTLJanitor{
		repo:   repo,
		logger: logger,
	}
}

// Start запускает TTL джанитор
func (j *TTLJanitor) Start(ctx context.Context) {
	ticker := time.NewTicker(1 * time.Minute) // Каждую минуту как указано в ТЗ
	defer ticker.Stop()

	j.logger.Info("TTL джанитор запущен")

	for {
		select {
		case <-ctx.Done():
			j.logger.Info("TTL джанитор остановлен")
			return
		case <-ticker.C:
			j.cleanupExpiredNotifications(ctx)
		}
	}
}

// cleanupExpiredNotifications удаляет просроченные уведомления для всех пользователей
func (j *TTLJanitor) cleanupExpiredNotifications(ctx context.Context) {
	start := time.Now()

	// Получаем все пользовательские ключи
	userKeys, err := j.repo.GetAllUserKeys(ctx)
	if err != nil {
		j.logger.Error("Ошибка получения пользовательских ключей", "error", err)
		return
	}

	if len(userKeys) == 0 {
		j.logger.Debug("Нет пользователей для очистки")
		return
	}

	var totalCleaned int64
	var processedUsers int

	// Обрабатываем каждого пользователя
	for _, userKey := range userKeys {
		// Парсим userKey для получения ID и логина
		userID, login, err := j.parseUserKey(userKey)
		if err != nil {
			j.logger.Warn("Ошибка парсинга user key", "user_key", userKey, "error", err)
			continue
		}

		// Очищаем просроченные уведомления для пользователя
		cleaned, err := j.repo.CleanupExpiredNotifications(ctx, userID, login, 100)
		if err != nil {
			j.logger.Error("Ошибка очистки просроченных уведомлений",
				"user_id", userID,
				"login", login,
				"error", err)
			continue
		}

		if cleaned > 0 {
			j.logger.Debug("Очищены просроченные уведомления",
				"user_id", userID,
				"login", login,
				"count", cleaned)
		}

		totalCleaned += cleaned
		processedUsers++

		// Небольшая пауза между пользователями чтобы не нагружать Redis
		select {
		case <-ctx.Done():
			return
		case <-time.After(10 * time.Millisecond):
		}
	}

	duration := time.Since(start)

	if totalCleaned > 0 || len(userKeys) > 10 {
		j.logger.Info("Завершена очистка просроченных уведомлений",
			"total_cleaned", totalCleaned,
			"processed_users", processedUsers,
			"total_users", len(userKeys),
			"duration", duration)
	} else {
		j.logger.Debug("Завершена очистка просроченных уведомлений",
			"total_cleaned", totalCleaned,
			"processed_users", processedUsers,
			"total_users", len(userKeys),
			"duration", duration)
	}
}

// parseUserKey парсит user key в формате "id-login"
func (j *TTLJanitor) parseUserKey(userKey string) (int64, string, error) {
	parts := strings.SplitN(userKey, "-", 2)
	if len(parts) != 2 {
		return 0, "", fmt.Errorf("неверный формат user key: %s", userKey)
	}

	userID, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		return 0, "", fmt.Errorf("ошибка парсинга user ID: %w", err)
	}

	return userID, parts[1], nil
}
