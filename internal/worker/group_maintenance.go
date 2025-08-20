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

// GroupMaintenance отвечает за обслуживание Consumer Groups
type GroupMaintenance struct {
	repo   domain.NotificationRepository
	logger *slog.Logger
}

// NewGroupMaintenance создает новый экземпляр GroupMaintenance
func NewGroupMaintenance(repo domain.NotificationRepository, logger *slog.Logger) *GroupMaintenance {
	return &GroupMaintenance{
		repo:   repo,
		logger: logger,
	}
}

// Start запускает воркер обслуживания групп
func (gm *GroupMaintenance) Start(ctx context.Context) {
	ticker := time.NewTicker(2 * time.Minute) // Каждые 2 минуты проверяем зависшие сообщения
	defer ticker.Stop()

	gm.logger.Info("Group maintenance воркер запущен")

	for {
		select {
		case <-ctx.Done():
			gm.logger.Info("Group maintenance воркер остановлен")
			return
		case <-ticker.C:
			gm.reclaimPendingMessages(ctx)
		}
	}
}

// reclaimPendingMessages перехватывает зависшие сообщения для всех пользователей
func (gm *GroupMaintenance) reclaimPendingMessages(ctx context.Context) {
	start := time.Now()

	// Получаем все пользовательские ключи
	userKeys, err := gm.repo.GetAllUserKeys(ctx)
	if err != nil {
		gm.logger.Error("Ошибка получения пользовательских ключей", "error", err)
		return
	}

	if len(userKeys) == 0 {
		gm.logger.Debug("Нет пользователей для обслуживания групп")
		return
	}

	var totalReclaimed int
	var processedUsers int

	// Обрабатываем каждого пользователя
	for _, userKey := range userKeys {
		// Парсим userKey для получения ID и логина
		userID, login, err := gm.parseUserKey(userKey)
		if err != nil {
			gm.logger.Warn("Ошибка парсинга user key", "user_key", userKey, "error", err)
			continue
		}

		// Перехватываем зависшие сообщения (idle больше 60 секунд как в ТЗ)
		reclaimedMessages, err := gm.repo.ReclaimPendingMessages(ctx, userID, login, 60*time.Second, 100)
		if err != nil {
			gm.logger.Error("Ошибка перехвата зависших сообщений",
				"user_id", userID,
				"login", login,
				"error", err)
			continue
		}

		if len(reclaimedMessages) > 0 {
			gm.logger.Info("Перехвачены зависшие сообщения",
				"user_id", userID,
				"login", login,
				"count", len(reclaimedMessages))

			// Здесь в полной реализации можно было бы попытаться
			// переотправить сообщения активному клиенту,
			// но для MVP просто логируем
		}

		totalReclaimed += len(reclaimedMessages)
		processedUsers++

		// Небольшая пауза между пользователями
		select {
		case <-ctx.Done():
			return
		case <-time.After(10 * time.Millisecond):
		}
	}

	duration := time.Since(start)

	if totalReclaimed > 0 || len(userKeys) > 10 {
		gm.logger.Info("Завершено обслуживание групп",
			"total_reclaimed", totalReclaimed,
			"processed_users", processedUsers,
			"total_users", len(userKeys),
			"duration", duration)
	} else {
		gm.logger.Debug("Завершено обслуживание групп",
			"total_reclaimed", totalReclaimed,
			"processed_users", processedUsers,
			"total_users", len(userKeys),
			"duration", duration)
	}
}

// parseUserKey парсит user key в формате "id-login"
func (gm *GroupMaintenance) parseUserKey(userKey string) (int64, string, error) {
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
