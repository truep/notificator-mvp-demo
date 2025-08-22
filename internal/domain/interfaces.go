package domain

import (
	"context"
	"time"
)

// NotificationRepository определяет интерфейс для работы с хранилищем уведомлений
type NotificationRepository interface {
	// CreateNotification создает уведомление для одного получателя
	CreateNotification(ctx context.Context, payload *NotificationPayload, target Target) (string, error)

	// GetNotification получает уведомление по ID
	GetNotification(ctx context.Context, notificationID string) (*NotificationPayload, error)

	// EnsureConsumerGroup создает Consumer Group если её нет
	EnsureConsumerGroup(ctx context.Context, userID int64, login string) error

	// ReadPendingMessages читает pending сообщения для consumer
	ReadPendingMessages(ctx context.Context, userID int64, login string, count int64) ([]StreamMessage, error)

	// ReadNewMessages читает новые сообщения из stream с блокировкой
	ReadNewMessages(
		ctx context.Context,
		userID int64,
		login string,
		blockTime time.Duration,
		count int64,
	) ([]StreamMessage, error)

	// AckMessage подтверждает прочтение сообщения и удаляет его
	AckMessage(ctx context.Context, userID int64, login string, streamID, notificationID string) error

	// CleanupExpiredNotifications удаляет просроченные уведомления
	CleanupExpiredNotifications(ctx context.Context, userID int64, login string, limit int64) (int64, error)

	// ReclaimPendingMessages перехватывает зависшие сообщения
	ReclaimPendingMessages(
		ctx context.Context,
		userID int64,
		login string,
		minIdleTime time.Duration,
		count int64,
	) ([]StreamMessage, error)

	// GetAllUserKeys возвращает все пользовательские ключи для джанитора
	GetAllUserKeys(ctx context.Context) ([]string, error)

	// SaveIdempotencyResult сохраняет результат для идемпотентности
	SaveIdempotencyResult(ctx context.Context, key string, result *NotifyResponse) error

	// GetIdempotencyResult получает сохраненный результат
	GetIdempotencyResult(ctx context.Context, key string) (*NotifyResponse, error)

	// GetPendingNotifications получает список pending уведомлений для пользователя
	GetPendingNotifications(ctx context.Context, userID int64, login string) ([]StreamMessage, error)

	// GetAllPendingNotifications получает pending уведомления для всех пользователей
	GetAllPendingNotifications(ctx context.Context) (map[string][]StreamMessage, error)

	// RangeLastMessages возвращает последние N сообщений из стрима пользователя
	RangeLastMessages(ctx context.Context, userID int64, login string, count int64) ([]StreamMessage, error)

	// GetReadStatuses возвращает статус прочтения для списка notification_id
	GetReadStatuses(ctx context.Context, userID int64, login string, notificationIDs []string) (map[string]bool, error)
}

// NotificationService определяет бизнес-логику работы с уведомлениями
type NotificationService interface {
	// CreateNotifications создает уведомления для списка получателей
	CreateNotifications(ctx context.Context, req *NotifyRequest, idempotencyKey string) (*NotifyResponse, error)

	// HandleWebSocketConnection обрабатывает WebSocket подключение клиента
	HandleWebSocketConnection(ctx context.Context, userID int64, login string, conn WebSocketConnection) error
}

// WebSocketConnection представляет интерфейс WebSocket соединения
type WebSocketConnection interface {
	ReadJSON(v interface{}) error
	WriteJSON(v interface{}) error
	Close() error
}

// StreamMessage представляет сообщение из Redis Stream
type StreamMessage struct {
	ID      string
	Fields  map[string]interface{}
	Payload *NotificationPayload // Загруженная полезная нагрузка
}
