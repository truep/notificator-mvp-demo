package repository

import (
	"context"
	"fmt"
	"log/slog"
	"strconv"
	"strings"

	"notification-mvp/internal/domain"
)

// GetPendingNotifications получает список pending уведомлений для пользователя
func (r *RedisRepository) GetPendingNotifications(
	ctx context.Context,
	userID int64,
	login string,
) ([]domain.StreamMessage, error) {
	// Просто вернем pending сообщения используя существующий метод
	return r.ReadPendingMessages(ctx, userID, login, 100)
}

// GetAllPendingNotifications получает pending уведомления для всех пользователей
func (r *RedisRepository) GetAllPendingNotifications(ctx context.Context) (map[string][]domain.StreamMessage, error) {
	// Получаем все пользовательские ключи
	userKeys, err := r.GetAllUserKeys(ctx)
	if err != nil {
		return nil, fmt.Errorf("ошибка получения пользовательских ключей: %w", err)
	}

	result := make(map[string][]domain.StreamMessage)

	for _, userKey := range userKeys {
		// Парсим userKey для получения ID и логина
		userID, login, err := parseUserKey(userKey)
		if err != nil {
			slog.Warn("Ошибка парсинга user key", "user_key", userKey, "error", err)
			continue
		}

		// Получаем pending уведомления для пользователя
		pendingMessages, err := r.GetPendingNotifications(ctx, userID, login)
		if err != nil {
			slog.Warn("Ошибка получения pending уведомлений",
				"user_id", userID, "login", login, "error", err)
			continue
		}

		if len(pendingMessages) > 0 {
			result[userKey] = pendingMessages
		}
	}

	return result, nil
}

// parseUserKey парсит user key в формате "id-login" (вспомогательная функция)
func parseUserKey(userKey string) (int64, string, error) {
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
