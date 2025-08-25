package repository

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"math/rand"
	"strconv"
	"strings"
	"time"

	"notification-mvp/internal/domain"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

// RedisRepository реализует интерфейс NotificationRepository
type RedisRepository struct {
	client *redis.Client
}

// NewRedisRepository создает новый экземпляр RedisRepository
func NewRedisRepository(client *redis.Client) *RedisRepository {
	return &RedisRepository{
		client: client,
	}
}

// CreateNotification создает уведомление для одного получателя
func (r *RedisRepository) CreateNotification(
	ctx context.Context,
	payload *domain.NotificationPayload,
	target domain.Target,
) (string, error) {
	// Генерируем UUID для уведомления
	notificationID := uuid.New().String()
	payload.NotificationID = notificationID
	payload.Target = target

	// Сериализуем payload в JSON
	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("ошибка сериализации payload: %w", err)
	}

	userKey := domain.UserKey(target.ID, target.Login)
	streamKey := domain.StreamKey(target.ID, target.Login)
	notificationKey := domain.NotificationKey(notificationID)
	ttlSchedulerKey := domain.TTLSchedulerKey(target.ID, target.Login)

	// Выполняем операции в пайплайне для атомарности
	pipe := r.client.Pipeline()

	// 1. Сохраняем payload с TTL = 15 минут
	pipe.Set(ctx, notificationKey, string(payloadBytes), domain.NotificationTTL)

	// 2. Убеждаемся что Consumer Group существует
	pipe.XGroupCreateMkStream(ctx, streamKey, domain.ConsumerGroupName, "$")

	// 3. Добавляем ссылку в персональный стрим
	xAddCmd := pipe.XAdd(ctx, &redis.XAddArgs{
		Stream: streamKey,
		MaxLen: 100,
		Approx: false,
		Values: map[string]interface{}{
			"nid":        notificationID,
			"created_at": payload.CreatedAt.Format(time.RFC3339),
		},
	})

	// Выполняем пайплайн
	_, err = pipe.Exec(ctx)
	if err != nil {
		// Игнорируем ошибку создания группы если она уже существует
		if !strings.Contains(err.Error(), "BUSYGROUP") {
			return "", fmt.Errorf("ошибка выполнения пайплайна: %w", err)
		}
	}

	// Получаем ID записи в стриме
	streamID, err := xAddCmd.Result()
	if err != nil {
		return "", fmt.Errorf("ошибка получения stream ID: %w", err)
	}

	// 4. Добавляем маркер истечения в ZSET планировщика
	expirationTime := time.Now().Add(domain.NotificationTTL).Unix()
	ttlEntry := domain.TTLSchedulerEntry(streamID, notificationID)

	err = r.client.ZAdd(ctx, ttlSchedulerKey, redis.Z{
		Score:  float64(expirationTime),
		Member: ttlEntry,
	}).Err()
	if err != nil {
		slog.Warn("Ошибка добавления в TTL планировщик", "error", err, "user", userKey)
		// Не критично для MVP, продолжаем
	}

	slog.Debug("Создано уведомление",
		"notification_id", notificationID,
		"stream_id", streamID,
		"user", userKey)

	return streamID, nil
}

// GetNotification получает уведомление по ID
func (r *RedisRepository) GetNotification(
	ctx context.Context,
	notificationID string,
) (*domain.NotificationPayload, error) {
	key := domain.NotificationKey(notificationID)

	payloadStr, err := r.client.Get(ctx, key).Result()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return nil, nil // Уведомление истекло или не существует
		}
		return nil, fmt.Errorf("ошибка получения уведомления: %w", err)
	}

	var payload domain.NotificationPayload
	if err := json.Unmarshal([]byte(payloadStr), &payload); err != nil {
		return nil, fmt.Errorf("ошибка десериализации payload: %w", err)
	}

	return &payload, nil
}

// EnsureConsumerGroup создает Consumer Group если её нет
func (r *RedisRepository) EnsureConsumerGroup(ctx context.Context, userID int64, login string) error {
	streamKey := domain.StreamKey(userID, login)

	err := r.client.XGroupCreateMkStream(ctx, streamKey, domain.ConsumerGroupName, "$").Err()
	if err != nil && !strings.Contains(err.Error(), "BUSYGROUP") {
		return fmt.Errorf("ошибка создания consumer group: %w", err)
	}

	return nil
}

// ReadPendingMessages читает pending сообщения для consumer
func (r *RedisRepository) ReadPendingMessages(
	ctx context.Context,
	userID int64,
	login string,
	count int64,
) ([]domain.StreamMessage, error) {
	streamKey := domain.StreamKey(userID, login)
	consumerID := domain.ConsumerID(userID)

	// Читаем pending сообщения (ID "0" возвращает все pending для данного consumer)
	streams, err := r.client.XReadGroup(ctx, &redis.XReadGroupArgs{
		Group:    domain.ConsumerGroupName,
		Consumer: consumerID,
		Streams:  []string{streamKey, "0"},
		Count:    count,
		Block:    0, // Не блокируем для pending
	}).Result()

	if err != nil {
		if err == redis.Nil {
			return []domain.StreamMessage{}, nil // Нет pending сообщений
		}
		return nil, fmt.Errorf("ошибка чтения pending сообщений: %w", err)
	}

	return r.convertRedisStreamsToMessages(ctx, streams)
}

// ReadNewMessages читает новые сообщения из stream с блокировкой
func (r *RedisRepository) ReadNewMessages(
	ctx context.Context,
	userID int64,
	login string,
	blockTime time.Duration,
	count int64,
) ([]domain.StreamMessage, error) {
	streamKey := domain.StreamKey(userID, login)
	consumerID := domain.ConsumerID(userID)

	// Читаем новые сообщения (ID ">" означает только новые, еще не доставленные)
	streams, err := r.client.XReadGroup(ctx, &redis.XReadGroupArgs{
		Group:    domain.ConsumerGroupName,
		Consumer: consumerID,
		Streams:  []string{streamKey, ">"},
		Count:    count,
		Block:    blockTime,
	}).Result()

	if err != nil {
		if err == redis.Nil {
			return []domain.StreamMessage{}, nil // Нет новых сообщений за время блокировки
		}
		return nil, fmt.Errorf("ошибка чтения новых сообщений: %w", err)
	}

	return r.convertRedisStreamsToMessages(ctx, streams)
}

// AckMessage подтверждает прочтение сообщения и удаляет его
func (r *RedisRepository) AckMessage(
	ctx context.Context,
	userID int64,
	login string,
	streamID, notificationID string,
) error {
	streamKey := domain.StreamKey(userID, login)
	stateKey := domain.NotificationStateKey(userID, login)

	// Выполняем операции в пайплайне
	pipe := r.client.Pipeline()

	// 1. Подтверждаем сообщение в Consumer Group (оставляем запись в стриме)
	pipe.XAck(ctx, streamKey, domain.ConsumerGroupName, streamID)

	// 2. Помечаем уведомление как прочитанное
	pipe.HSet(ctx, stateKey, notificationID, "read")

	_, err := pipe.Exec(ctx)
	if err != nil {
		return fmt.Errorf("ошибка подтверждения сообщения: %w", err)
	}

	slog.Debug("Помечено прочтение уведомления",
		"notification_id", notificationID,
		"stream_id", streamID,
		"user", domain.UserKey(userID, login))

	return nil
}

// AcquireConsumerLock пытается получить эксклюзивную блокировку чтения для пользователя
func (r *RedisRepository) AcquireConsumerLock(
	ctx context.Context,
	userID int64,
	login string,
	podID string,
	ttl time.Duration,
) (bool, error) {
	key := domain.ConsumerLockKey(userID, login)
	ok, err := r.client.SetNX(ctx, key, podID, ttl).Result()
	if err != nil {
		return false, fmt.Errorf("ошибка установки consumer lock: %w", err)
	}
	return ok, nil
}

// RenewConsumerLock продлевает блокировку если она принадлежит podID
func (r *RedisRepository) RenewConsumerLock(
	ctx context.Context,
	userID int64,
	login string,
	podID string,
	ttl time.Duration,
) (bool, error) {
	key := domain.ConsumerLockKey(userID, login)
	val, err := r.client.Get(ctx, key).Result()
	if err != nil {
		if err == redis.Nil {
			return false, nil
		}
		return false, fmt.Errorf("ошибка чтения consumer lock: %w", err)
	}
	if val != podID {
		return false, nil
	}
	// продляем с небольшой джиттер-защитой от дребезга
	extend := ttl + time.Duration(rand.Intn(250))*time.Millisecond
	if err := r.client.PExpire(ctx, key, extend).Err(); err != nil {
		return false, fmt.Errorf("ошибка продления consumer lock: %w", err)
	}
	return true, nil
}

// ReleaseConsumerLock снимает блокировку если она принадлежит podID
func (r *RedisRepository) ReleaseConsumerLock(
	ctx context.Context,
	userID int64,
	login string,
	podID string,
) error {
	key := domain.ConsumerLockKey(userID, login)
	val, err := r.client.Get(ctx, key).Result()
	if err != nil {
		if err == redis.Nil {
			return nil
		}
		return fmt.Errorf("ошибка чтения consumer lock: %w", err)
	}
	if val != podID {
		return nil
	}
	if err := r.client.Del(ctx, key).Err(); err != nil {
		return fmt.Errorf("ошибка удаления consumer lock: %w", err)
	}
	return nil
}

// CleanupExpiredNotifications удаляет просроченные уведомления
func (r *RedisRepository) CleanupExpiredNotifications(
	ctx context.Context,
	userID int64,
	login string,
	limit int64,
) (int64, error) {
	ttlSchedulerKey := domain.TTLSchedulerKey(userID, login)
	streamKey := domain.StreamKey(userID, login)
	now := time.Now().Unix()

	// Получаем просроченные записи
	expired, err := r.client.ZRangeByScoreWithScores(ctx, ttlSchedulerKey, &redis.ZRangeBy{
		Min:   "-inf",
		Max:   fmt.Sprintf("%d", now),
		Count: limit,
	}).Result()

	if err != nil {
		return 0, fmt.Errorf("ошибка получения просроченных записей: %w", err)
	}

	if len(expired) == 0 {
		return 0, nil
	}

	var cleaned int64
	pipe := r.client.Pipeline()

	for _, z := range expired {
		entry := z.Member.(string)
		parts := strings.Split(entry, "|")
		if len(parts) != 2 {
			continue
		}

		streamID := parts[0]
		notificationID := parts[1]
		notificationKey := domain.NotificationKey(notificationID)

		// Подтверждаем и удаляем сообщение
		pipe.XAck(ctx, streamKey, domain.ConsumerGroupName, streamID)
		pipe.XDel(ctx, streamKey, streamID)
		pipe.Del(ctx, notificationKey)

		cleaned++
	}

	// Удаляем обработанные записи из планировщика
	members := make([]interface{}, len(expired))
	for i, z := range expired {
		members[i] = z.Member
	}
	pipe.ZRem(ctx, ttlSchedulerKey, members...)

	_, err = pipe.Exec(ctx)
	if err != nil {
		return 0, fmt.Errorf("ошибка очистки просроченных уведомлений: %w", err)
	}

	slog.Debug("Очищены просроченные уведомления",
		"user", domain.UserKey(userID, login),
		"count", cleaned)

	return cleaned, nil
}

// ReclaimPendingMessages перехватывает зависшие сообщения
func (r *RedisRepository) ReclaimPendingMessages(
	ctx context.Context,
	userID int64,
	login string,
	minIdleTime time.Duration,
	count int64,
) ([]domain.StreamMessage, error) {
	streamKey := domain.StreamKey(userID, login)
	consumerID := domain.ConsumerID(userID)
	minIdleMs := minIdleTime.Milliseconds()

	// Используем XAUTOCLAIM для перехвата зависших сообщений
	result, _, err := r.client.XAutoClaim(ctx, &redis.XAutoClaimArgs{
		Stream:   streamKey,
		Group:    domain.ConsumerGroupName,
		Consumer: consumerID,
		MinIdle:  time.Duration(minIdleMs) * time.Millisecond,
		Start:    "0-0",
		Count:    count,
	}).Result()

	if err != nil {
		return nil, fmt.Errorf("ошибка перехвата зависших сообщений: %w", err)
	}

	// Конвертируем результат в наш формат
	var messages []domain.StreamMessage
	for _, msg := range result {
		streamMsg := domain.StreamMessage{
			ID:     msg.ID,
			Fields: msg.Values,
		}

		// Загружаем payload если есть nid
		if nidStr, ok := msg.Values["nid"].(string); ok {
			payload, err := r.GetNotification(ctx, nidStr)
			if err != nil {
				slog.Warn("Ошибка загрузки payload при перехвате", "error", err, "nid", nidStr)
			}
			streamMsg.Payload = payload
		}

		messages = append(messages, streamMsg)
	}

	if len(messages) > 0 {
		slog.Debug("Перехвачены зависшие сообщения",
			"user", domain.UserKey(userID, login),
			"count", len(messages))
	}

	return messages, nil
}

// GetAllUserKeys возвращает все пользовательские ключи для джанитора
func (r *RedisRepository) GetAllUserKeys(ctx context.Context) ([]string, error) {
	// Сканируем все ключи TTL планировщика
	var allKeys []string
	var cursor uint64

	for {
		keys, newCursor, err := r.client.Scan(ctx, cursor, domain.TTLSchedulerKeyPrefix+"*", 100).Result()
		if err != nil {
			return nil, fmt.Errorf("ошибка сканирования ключей: %w", err)
		}

		// Извлекаем user ключи из имен
		for _, key := range keys {
			if strings.HasPrefix(key, domain.TTLSchedulerKeyPrefix) {
				userKey := strings.TrimPrefix(key, domain.TTLSchedulerKeyPrefix)
				allKeys = append(allKeys, userKey)
			}
		}

		cursor = newCursor
		if cursor == 0 {
			break
		}
	}

	return allKeys, nil
}

// SaveIdempotencyResult сохраняет результат для идемпотентности
func (r *RedisRepository) SaveIdempotencyResult(ctx context.Context, key string, result *domain.NotifyResponse) error {
	redisKey := domain.IdempotencyKey(key)

	resultBytes, err := json.Marshal(result)
	if err != nil {
		return fmt.Errorf("ошибка сериализации результата идемпотентности: %w", err)
	}

	// Сохраняем на 10 минут как указано в ТЗ
	err = r.client.Set(ctx, redisKey, string(resultBytes), 10*time.Minute).Err()
	if err != nil {
		return fmt.Errorf("ошибка сохранения результата идемпотентности: %w", err)
	}

	return nil
}

// GetIdempotencyResult получает сохраненный результат
func (r *RedisRepository) GetIdempotencyResult(ctx context.Context, key string) (*domain.NotifyResponse, error) {
	redisKey := domain.IdempotencyKey(key)

	resultStr, err := r.client.Get(ctx, redisKey).Result()
	if err != nil {
		if err == redis.Nil {
			return nil, nil // Результат не найден
		}
		return nil, fmt.Errorf("ошибка получения результата идемпотентности: %w", err)
	}

	var result domain.NotifyResponse
	if err := json.Unmarshal([]byte(resultStr), &result); err != nil {
		return nil, fmt.Errorf("ошибка десериализации результата идемпотентности: %w", err)
	}

	return &result, nil
}

// convertRedisStreamsToMessages конвертирует ответ Redis в наш формат
func (r *RedisRepository) convertRedisStreamsToMessages(
	ctx context.Context,
	streams []redis.XStream,
) ([]domain.StreamMessage, error) {
	var messages []domain.StreamMessage

	for _, stream := range streams {
		for _, msg := range stream.Messages {
			streamMsg := domain.StreamMessage{
				ID:     msg.ID,
				Fields: msg.Values,
			}

			// Загружаем payload уведомления если есть nid
			if nidStr, ok := msg.Values["nid"].(string); ok {
				payload, err := r.GetNotification(ctx, nidStr)
				if err != nil {
					slog.Warn("Ошибка загрузки payload", "error", err, "nid", nidStr)
					continue
				}
				streamMsg.Payload = payload
			}

			messages = append(messages, streamMsg)
		}
	}

	return messages, nil
}

// RangeLastMessages возвращает последние count сообщений из стрима пользователя (без отметки о прочтении)
func (r *RedisRepository) RangeLastMessages(
	ctx context.Context,
	userID int64,
	login string,
	count int64,
) ([]domain.StreamMessage, error) {
	streamKey := domain.StreamKey(userID, login)

	msgs, err := r.client.XRevRangeN(ctx, streamKey, "+", "-", int64(count)).Result()
	if err != nil {
		if err == redis.Nil {
			return []domain.StreamMessage{}, nil
		}
		return nil, fmt.Errorf("ошибка XRANGE: %w", err)
	}

	// Разворачиваем в хронологическом порядке (от старых к новым)
	for i, j := 0, len(msgs)-1; i < j; i, j = i+1, j-1 {
		msgs[i], msgs[j] = msgs[j], msgs[i]
	}

	// Конвертируем и подгружаем payload
	var messages []domain.StreamMessage
	for _, m := range msgs {
		sm := domain.StreamMessage{ID: m.ID, Fields: m.Values}
		if nidStr, ok := m.Values["nid"].(string); ok {
			payload, err := r.GetNotification(ctx, nidStr)
			if err != nil {
				slog.Warn("Ошибка загрузки payload", "error", err, "nid", nidStr)
			}
			sm.Payload = payload
		}
		messages = append(messages, sm)
	}

	return messages, nil
}

// GetReadStatuses возвращает признак прочтения для списка notification_id
func (r *RedisRepository) GetReadStatuses(
	ctx context.Context,
	userID int64,
	login string,
	notificationIDs []string,
) (map[string]bool, error) {
	stateKey := domain.NotificationStateKey(userID, login)
	result := make(map[string]bool, len(notificationIDs))
	if len(notificationIDs) == 0 {
		return result, nil
	}

	vals, err := r.client.HMGet(ctx, stateKey, notificationIDs...).Result()
	if err != nil {
		return nil, fmt.Errorf("ошибка HMGET статусов: %w", err)
	}

	for i, v := range vals {
		read := false
		if s, ok := v.(string); ok && s == "read" {
			read = true
		}
		result[notificationIDs[i]] = read
	}

	return result, nil
}

// SetUserRetentionDays сохраняет срок хранения для пользователя (1..15 дней)
func (r *RedisRepository) SetUserRetentionDays(ctx context.Context, userID int64, login string, days int) error {
	key := domain.RetentionKey(userID, login)
	if days < 1 {
		days = 1
	}
	if days > 15 {
		days = 15
	}
	return r.client.Set(ctx, key, fmt.Sprintf("%d", days), 0).Err()
}

// GetUserRetentionDays возвращает срок хранения в днях (дефолт 7)
func (r *RedisRepository) GetUserRetentionDays(ctx context.Context, userID int64, login string) (int, error) {
	key := domain.RetentionKey(userID, login)
	val, err := r.client.Get(ctx, key).Result()
	if err != nil {
		if err == redis.Nil {
			return 7, nil
		}
		return 7, fmt.Errorf("ошибка чтения retention: %w", err)
	}
	d, convErr := strconv.Atoi(val)
	if convErr != nil {
		return 7, nil
	}
	if d < 1 {
		d = 1
	} else if d > 15 {
		d = 15
	}
	return d, nil
}

// TrimUserStreamByRetention делает XTRIM MINID по времени в зависимости от retention
func (r *RedisRepository) TrimUserStreamByRetention(ctx context.Context, userID int64, login string) error {
	days, err := r.GetUserRetentionDays(ctx, userID, login)
	if err != nil {
		return err
	}
	cutoff := time.Now().Add(-time.Duration(days) * 24 * time.Hour)
	minID := fmt.Sprintf("%d-0", cutoff.UnixMilli())
	streamKey := domain.StreamKey(userID, login)
	// MINID доступен в Redis 7.0+
	return r.client.XTrimMinID(ctx, streamKey, minID).Err()
}
