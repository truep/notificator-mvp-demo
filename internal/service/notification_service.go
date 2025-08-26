package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"notification-mvp/internal/domain"
	"notification-mvp/internal/metrics"
)

// NotificationService реализует бизнес-логику работы с уведомлениями
type NotificationService struct {
	repo   domain.NotificationRepository
	logger *slog.Logger
	podID  string
}

// NewNotificationService создает новый экземпляр NotificationService
func NewNotificationService(repo domain.NotificationRepository, logger *slog.Logger) *NotificationService {
	return &NotificationService{
		repo:   repo,
		logger: logger,
	}
}

// WithPodID задает идентификатор текущего pod для распределенной блокировки
func (s *NotificationService) WithPodID(podID string) *NotificationService {
	s.podID = podID
	return s
}

// CreateNotifications создает уведомления для списка получателей
func (s *NotificationService) CreateNotifications(
	ctx context.Context,
	req *domain.NotifyRequest,
	idempotencyKey string,
) (*domain.NotifyResponse, error) {
	// Проверяем идемпотентность если ключ предоставлен
	if idempotencyKey != "" {
		if cached, err := s.repo.GetIdempotencyResult(ctx, idempotencyKey); err == nil && cached != nil {
			s.logger.Debug("Возвращен кэшированный результат", "idempotency_key", idempotencyKey)
			return cached, nil
		}
	}

	// Валидируем запрос
	if err := s.validateNotifyRequest(req); err != nil {
		return nil, fmt.Errorf("ошибка валидации запроса: %w", err)
	}

	var results []domain.NotifyResult

	// Создаем уведомления для каждого получателя
	for _, target := range req.Target {
		// Создаем базовый payload
		payload := &domain.NotificationPayload{
			Message:   req.Message,
			CreatedAt: req.CreatedAt,
			Source:    req.Source,
			Target:    target,
		}

		// Создаем уведомление в репозитории
		streamID, err := s.repo.CreateNotification(ctx, payload, target)
		if err != nil {
			s.logger.Error("Ошибка создания уведомления",
				"error", err,
				"target_id", target.ID,
				"target_login", target.Login)
			continue // Продолжаем с остальными получателями
		}

		result := domain.NotifyResult{
			Target:         target,
			NotificationID: payload.NotificationID,
		}
		results = append(results, result)

		s.logger.Debug("Создано уведомление",
			"notification_id", payload.NotificationID,
			"stream_id", streamID,
			"target_id", target.ID,
			"target_login", target.Login)
	}

	response := &domain.NotifyResponse{
		Results: results,
	}

	// Сохраняем результат для идемпотентности
	if idempotencyKey != "" {
		if err := s.repo.SaveIdempotencyResult(ctx, idempotencyKey, response); err != nil {
			s.logger.Warn("Ошибка сохранения результата идемпотентности", "error", err)
			// Не критично, продолжаем
		}
	}

	s.logger.Info("Созданы уведомления",
		"count", len(results),
		"total_targets", len(req.Target),
		"source", req.Source)

	return response, nil
}

// HandleWebSocketConnection обрабатывает WebSocket подключение клиента
func (s *NotificationService) HandleWebSocketConnection(
	ctx context.Context,
	userID int64,
	login string,
	conn domain.WebSocketConnection,
) error {
	s.logger.Info("Новое WebSocket подключение", "user_id", userID, "login", login)

	// Убеждаемся что Consumer Group существует
	if err := s.repo.EnsureConsumerGroup(ctx, userID, login); err != nil {
		return fmt.Errorf("ошибка создания consumer group: %w", err)
	}

	// Создаем контекст с отменой для горутин
	wsCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	// Канал для ошибок
	errChan := make(chan error, 2)

	// Захватываем consumer-lock (если задан podID)
	const lockTTL = 60 * time.Second
	if s.podID != "" {
		ok, err := s.repo.AcquireConsumerLock(wsCtx, userID, login, s.podID, lockTTL)
		if err != nil {
			s.logger.Warn("Ошибка захвата consumer-lock", "error", err, "user_id", userID, "login", login)
		} else if !ok {
			s.logger.Info("Consumer-lock уже занят другим pod — продолжаем только локальную доставку", "user_id", userID, "login", login)
		} else {
			s.logger.Debug("Захвачен consumer-lock", "user_id", userID, "login", login, "pod", s.podID)
			// Продление lock в фоне
			go func() {
				ticker := time.NewTicker(20 * time.Second)
				defer ticker.Stop()
				for {
					select {
					case <-wsCtx.Done():
						return
					case <-ticker.C:
						if ok, _ := s.repo.RenewConsumerLock(wsCtx, userID, login, s.podID, lockTTL); !ok {
							// Потеряли lock
							s.logger.Info("Потерян consumer-lock при продлении", "user_id", userID, "login", login)
							return
						}
					}
				}
			}()
			defer func() { _ = s.repo.ReleaseConsumerLock(context.Background(), userID, login, s.podID) }()
		}
	}

	// Горутина для чтения сообщений от клиента (обработка ACK)
	go s.handleClientMessages(wsCtx, userID, login, conn, errChan)

	// Горутина для отправки уведомлений клиенту
	go s.handleNotificationDelivery(wsCtx, userID, login, conn, errChan)

	// Ждем первую ошибку или завершение контекста
	select {
	case err := <-errChan:
		if err != nil {
			s.logger.Error("Ошибка WebSocket соединения", "error", err, "user_id", userID, "login", login)
		}
		return err
	case <-ctx.Done():
		s.logger.Info("WebSocket соединение завершено", "user_id", userID, "login", login)
		return ctx.Err()
	}
}

// handleClientMessages обрабатывает сообщения от клиента (ACK)
func (s *NotificationService) handleClientMessages(
	ctx context.Context,
	userID int64,
	login string,
	conn domain.WebSocketConnection,
	errChan chan<- error,
) {
	for {
		select {
		case <-ctx.Done():
			return
		default:
			var raw domain.WebSocketMessage
			if err := conn.ReadJSON(&raw); err != nil {
				errChan <- fmt.Errorf("ошибка чтения сообщения от клиента: %w", err)
				return
			}

			switch raw.Type {
			case domain.MessageTypeNotificationRead:
				var read domain.ReadEvent
				read.Type = raw.Type
				if m, ok := raw.Data.(map[string]interface{}); ok {
					if v, ok := m["notification_id"].(string); ok {
						read.Data.NotificationID = v
					}
					if v, ok := m["stream_id"].(string); ok {
						read.Data.StreamID = v
					}
				}
				if err := s.handleReadAck(ctx, userID, login, &read, conn); err != nil {
					s.logger.Error("Ошибка обработки ACK", "error", err)
				}
			case domain.MessageTypeRetentionSet:
				var ev domain.RetentionSetEvent
				ev.Type = raw.Type
				if m, ok := raw.Data.(map[string]interface{}); ok {
					if v, ok := m["days"].(float64); ok {
						ev.Data.Days = int(v)
					}
				}
				if ev.Data.Days >= 1 && ev.Data.Days <= 15 {
					if err := s.repo.SetUserRetentionDays(ctx, userID, login, ev.Data.Days); err != nil {
						s.logger.Warn("Ошибка установки retention", "error", err)
					}
				}
			case domain.MessageTypeSyncRequest:
				var ev domain.SyncRequestEvent
				ev.Type = raw.Type
				if m, ok := raw.Data.(map[string]interface{}); ok {
					if v, ok := m["limit"].(float64); ok {
						ev.Data.Limit = int(v)
					}
				}
				if ev.Data.Limit <= 0 || ev.Data.Limit > 1000 {
					ev.Data.Limit = 100
				}
				if err := s.deliverLastMessagesWithRead(ctx, userID, login, conn, int64(ev.Data.Limit)); err != nil {
					s.logger.Warn("Ошибка sync.request", "error", err)
				}
			default:
				// игнорируем неизвестные
			}
		}
	}
}

// handleNotificationDelivery доставляет уведомления клиенту
func (s *NotificationService) handleNotificationDelivery(
	ctx context.Context,
	userID int64,
	login string,
	conn domain.WebSocketConnection,
	errChan chan<- error,
) {
	// Сначала отправляем все pending уведомления
	if err := s.deliverPendingMessages(ctx, userID, login, conn); err != nil {
		errChan <- fmt.Errorf("ошибка доставки pending сообщений: %w", err)
		return
	}

	// Затем отправляем последние 100 сообщений из истории с признаком прочтения
	if err := s.deliverLastMessagesWithRead(ctx, userID, login, conn, 100); err != nil {
		s.logger.Warn("Ошибка начальной синхронизации XRANGE", "error", err)
	}

	// Затем слушаем новые уведомления в цикле
	for {
		select {
		case <-ctx.Done():
			return
		default:
			// Читаем новые сообщения с блокировкой 30 секунд (как в ТЗ)
			messages, err := s.repo.ReadNewMessages(ctx, userID, login, 30*time.Second, 100)
			if err != nil {
				errChan <- fmt.Errorf("ошибка чтения новых сообщений: %w", err)
				return
			}

			// Отправляем полученные сообщения
			for _, msg := range messages {
				if err := s.sendMessageToClient(conn, &msg); err != nil {
					errChan <- fmt.Errorf("ошибка отправки сообщения клиенту: %w", err)
					return
				}
			}
		}
	}
}

// deliverPendingMessages доставляет все pending сообщения клиенту
func (s *NotificationService) deliverPendingMessages(
	ctx context.Context,
	userID int64,
	login string,
	conn domain.WebSocketConnection,
) error {
	messages, err := s.repo.ReadPendingMessages(ctx, userID, login, 100)
	if err != nil {
		return fmt.Errorf("ошибка чтения pending сообщений: %w", err)
	}

	s.logger.Debug("Доставляем pending сообщения", "count", len(messages), "user_id", userID, "login", login)

	for _, msg := range messages {
		if err := s.sendMessageToClient(conn, &msg); err != nil {
			return fmt.Errorf("ошибка отправки pending сообщения: %w", err)
		}
	}

	return nil
}

// deliverLastMessagesWithRead отправляет последние N сообщений с признаком read/unread
func (s *NotificationService) deliverLastMessagesWithRead(
	ctx context.Context,
	userID int64,
	login string,
	conn domain.WebSocketConnection,
	count int64,
) error {
	messages, err := s.repo.RangeLastMessages(ctx, userID, login, count)
	if err != nil {
		return err
	}

	// Собираем список notification_id
	ids := make([]string, 0, len(messages))
	for _, m := range messages {
		if m.Payload != nil {
			ids = append(ids, m.Payload.NotificationID)
		}
	}

	readMap, err := s.repo.GetReadStatuses(ctx, userID, login, ids)
	if err != nil {
		return err
	}

	for _, msg := range messages {
		// Мы хотим отправить, даже если payload истёк (auto_cleared)
		if err := s.sendMessageToClientWithRead(conn, &msg, readMap); err != nil {
			return err
		}
	}
	return nil
}

// sendMessageToClientWithRead как sendMessageToClient, но выставляет Read
func (s *NotificationService) sendMessageToClientWithRead(
	conn domain.WebSocketConnection,
	msg *domain.StreamMessage,
	readMap map[string]bool,
) error {
	var pushPayload domain.PushPayload
	var read bool

	if msg.Payload != nil {
		read = readMap[msg.Payload.NotificationID]
		pushPayload = domain.PushPayload{
			NotificationID: msg.Payload.NotificationID,
			StreamID:       msg.ID,
			Message:        msg.Payload.Message,
			CreatedAt:      msg.Payload.CreatedAt,
			Source:         msg.Payload.Source,
			Status:         domain.StatusUnread,
			Read:           read,
		}
	} else {
		// Истёкший payload
		nid := ""
		if nidVal, ok := msg.Fields["nid"]; ok {
			if nidStr, ok := nidVal.(string); ok {
				nid = nidStr
			}
		}
		pushPayload = domain.PushPayload{
			NotificationID: nid,
			StreamID:       msg.ID,
			Status:         domain.StatusAutoCleared,
			Read:           true,
		}
	}

	pushMessage := domain.PushMessage{Type: domain.MessageTypeNotificationPush, Data: pushPayload}
	if err := conn.WriteJSON(pushMessage); err != nil {
		return fmt.Errorf("ошибка отправки сообщения в WebSocket: %w", err)
	}
	return nil
}

// sendMessageToClient отправляет сообщение клиенту через WebSocket
func (s *NotificationService) sendMessageToClient(conn domain.WebSocketConnection, msg *domain.StreamMessage) error {
	var pushPayload domain.PushPayload
	var status string

	if msg.Payload != nil {
		// Уведомление существует
		pushPayload = domain.PushPayload{
			NotificationID: msg.Payload.NotificationID,
			StreamID:       msg.ID,
			Message:        msg.Payload.Message,
			CreatedAt:      msg.Payload.CreatedAt,
			Source:         msg.Payload.Source,
			Status:         domain.StatusUnread,
			Read:           false,
		}
		status = domain.StatusUnread
		// метрики
		metrics.NotificationsSent.Inc()
		latency := time.Since(msg.Payload.CreatedAt).Milliseconds()
		if latency > 0 {
			metrics.DeliveryLatencyMs.Observe(float64(latency))
		}
	} else {
		// Payload отсутствует - уведомление истекло
		nid := ""
		if nidVal, ok := msg.Fields["nid"]; ok {
			if nidStr, ok := nidVal.(string); ok {
				nid = nidStr
			}
		}

		pushPayload = domain.PushPayload{
			NotificationID: nid,
			StreamID:       msg.ID,
			Status:         domain.StatusAutoCleared,
			Read:           true,
		}
		status = domain.StatusAutoCleared
		metrics.NotificationsAutoCleared.Inc()
	}

	pushMessage := domain.PushMessage{
		Type: domain.MessageTypeNotificationPush,
		Data: pushPayload,
	}

	if err := conn.WriteJSON(pushMessage); err != nil {
		return fmt.Errorf("ошибка отправки сообщения в WebSocket: %w", err)
	}

	s.logger.Debug("Отправлено уведомление клиенту",
		"notification_id", pushPayload.NotificationID,
		"stream_id", pushPayload.StreamID,
		"status", status)

	return nil
}

// handleReadAck обрабатывает подтверждение прочтения от клиента
func (s *NotificationService) handleReadAck(
	ctx context.Context,
	userID int64,
	login string,
	readEvent *domain.ReadEvent,
	conn domain.WebSocketConnection,
) error {
	if readEvent.Type != domain.MessageTypeNotificationRead {
		return fmt.Errorf("неожиданный тип сообщения: %s", readEvent.Type)
	}

	// Подтверждаем и удаляем сообщение
	err := s.repo.AckMessage(ctx, userID, login, readEvent.Data.StreamID, readEvent.Data.NotificationID)
	if err != nil {
		return fmt.Errorf("ошибка подтверждения сообщения: %w", err)
	}

	// Отправляем ACK клиенту
	ackMessage := domain.WebSocketMessage{
		Type: domain.MessageTypeNotificationAck,
		Data: map[string]string{
			"notification_id": readEvent.Data.NotificationID,
			"stream_id":       readEvent.Data.StreamID,
		},
	}

	if err := conn.WriteJSON(ackMessage); err != nil {
		return fmt.Errorf("ошибка отправки ACK: %w", err)
	}
	metrics.NotificationsAcked.Inc()

	s.logger.Debug("Обработан ACK от клиента",
		"notification_id", readEvent.Data.NotificationID,
		"stream_id", readEvent.Data.StreamID,
		"user_id", userID,
		"login", login)

	return nil
}

// validateNotifyRequest валидирует входящий запрос
func (s *NotificationService) validateNotifyRequest(req *domain.NotifyRequest) error {
	if req == nil {
		return fmt.Errorf("запрос не может быть nil")
	}

	if len(req.Target) == 0 {
		return fmt.Errorf("список получателей не может быть пустым")
	}

	if req.Message == "" {
		return fmt.Errorf("сообщение не может быть пустым")
	}

	if req.Source == "" {
		return fmt.Errorf("источник не может быть пустым")
	}

	if req.CreatedAt.IsZero() {
		return fmt.Errorf("время создания не может быть нулевым")
	}

	for i, target := range req.Target {
		if target.ID <= 0 {
			return fmt.Errorf("ID получателя %d должен быть положительным", i)
		}
		if target.Login == "" {
			return fmt.Errorf("логин получателя %d не может быть пустым", i)
		}
	}

	return nil
}

// generateIdempotencyHash генерирует хеш для идемпотентности на основе содержимого запроса
func (s *NotificationService) generateIdempotencyHash(req *domain.NotifyRequest) (string, error) {
	// Сериализуем запрос для хеширования
	data, err := json.Marshal(req)
	if err != nil {
		return "", fmt.Errorf("ошибка сериализации запроса для хеширования: %w", err)
	}

	hash := sha256.Sum256(data)
	return hex.EncodeToString(hash[:]), nil
}
