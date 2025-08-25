package domain

import (
	"fmt"
	"time"
)

// NotifyRequest представляет входящий запрос на создание уведомления
type NotifyRequest struct {
	Target    []Target  `json:"target"`
	Message   string    `json:"message"`
	CreatedAt time.Time `json:"created_at"`
	Source    string    `json:"source"`
}

// Target представляет получателя уведомления
type Target struct {
	ID    int64  `json:"id"`
	Login string `json:"login"`
}

// NotificationPayload представляет полезную нагрузку уведомления для хранения в Redis
type NotificationPayload struct {
	NotificationID string    `json:"notification_id"`
	Message        string    `json:"message"`
	CreatedAt      time.Time `json:"created_at"`
	Source         string    `json:"source"`
	Target         Target    `json:"target"`
}

// PushMessage представляет сообщение для отправки клиенту через WebSocket
type PushMessage struct {
	Type string      `json:"type"`
	Data PushPayload `json:"data"`
}

// PushPayload представляет данные уведомления для клиента
type PushPayload struct {
	NotificationID string    `json:"notification_id"`
	StreamID       string    `json:"stream_id"`
	Message        string    `json:"message,omitempty"`
	CreatedAt      time.Time `json:"created_at"`
	Source         string    `json:"source"`
	Status         string    `json:"status"`
	Read           bool      `json:"read"`
}

// ReadEvent представляет событие прочтения от клиента
type ReadEvent struct {
	Type string   `json:"type"`
	Data ReadData `json:"data"`
}

// ReadData представляет данные события прочтения
type ReadData struct {
	NotificationID string `json:"notification_id"`
	StreamID       string `json:"stream_id"`
}

// NotifyResponse представляет ответ на запрос создания уведомления
type NotifyResponse struct {
	Results []NotifyResult `json:"results"`
}

// NotifyResult представляет результат создания уведомления для одного получателя
type NotifyResult struct {
	Target         Target `json:"target"`
	NotificationID string `json:"notification_id"`
}

// WebSocketMessage представляет общий формат сообщения WebSocket
type WebSocketMessage struct {
	Type string      `json:"type"`
	Data interface{} `json:"data"`
}

// RetentionSetEvent задает новый срок хранения (в днях)
type RetentionSetEvent struct {
	Type string           `json:"type"`
	Data RetentionSetData `json:"data"`
}

type RetentionSetData struct {
	Days int `json:"days"`
}

// SyncRequestEvent запрашивает последние N событий
type SyncRequestEvent struct {
	Type string          `json:"type"`
	Data SyncRequestData `json:"data"`
}

type SyncRequestData struct {
	Limit int `json:"limit"`
}

// SyncResponse содержит последние события и их статусы
type SyncResponse struct {
	Type string           `json:"type"`
	Data []map[string]any `json:"data"`
}

// Constants для типов сообщений
const (
	MessageTypeNotificationPush = "notification.push"
	MessageTypeNotificationRead = "notification.read"
	MessageTypeNotificationAck  = "notification.read.ack"
	MessageTypeRetentionSet     = "retention.set"
	MessageTypeSyncRequest      = "sync.request"
	MessageTypeSyncResponse     = "sync.response"
	MessageTypeError            = "error"

	StatusUnread      = "unread"
	StatusAutoCleared = "auto_cleared"
)

// Constants для Redis ключей
const (
	StreamKeyPrefix       = "stream:user:"
	NotificationKeyPrefix = "notification:"
	TTLSchedulerKeyPrefix = "notif:ttl:"
	IdempotencyKeyPrefix  = "notify:req:"

	NotificationStateKeyPrefix = "notification_state:"
	ConsumerLockKeyPrefix      = "notif:lock:consumer:"
	RetentionKeyPrefix         = "notif:retention:"

	ConsumerGroupName = "notifications"
	NotificationTTL   = 15 * time.Minute // 15 минут как указано в ТЗ
)

// Константы для формирования ключей
func StreamKey(userID int64, login string) string {
	return StreamKeyPrefix + UserKey(userID, login)
}

func NotificationKey(uuid string) string {
	return NotificationKeyPrefix + uuid
}

func TTLSchedulerKey(userID int64, login string) string {
	return TTLSchedulerKeyPrefix + UserKey(userID, login)
}

func IdempotencyKey(key string) string {
	return IdempotencyKeyPrefix + key
}

func UserKey(userID int64, login string) string {
	return fmt.Sprintf("%d-%s", userID, login)
}

func ConsumerID(userID int64) string {
	return fmt.Sprintf("user:%d", userID)
}

// TTLSchedulerEntry представляет запись в ZSET планировщика TTL
func TTLSchedulerEntry(streamID, notificationID string) string {
	return streamID + "|" + notificationID
}

// NotificationStateKey возвращает ключ хэша статусов прочтения пользователя
func NotificationStateKey(userID int64, login string) string {
	return NotificationStateKeyPrefix + UserKey(userID, login)
}

// ConsumerLockKey возвращает ключ блокировки consumer для пользователя
func ConsumerLockKey(userID int64, login string) string {
	return ConsumerLockKeyPrefix + UserKey(userID, login)
}

// RetentionKey возвращает ключ хранения персистентного TTL профиля пользователя
func RetentionKey(userID int64, login string) string {
	return RetentionKeyPrefix + UserKey(userID, login)
}
