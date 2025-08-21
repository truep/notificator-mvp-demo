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
)

// NotificationService реализует бизнес-логику работы с уведомлениями
type NotificationService struct {
	repo   domain.NotificationRepository
	logger *slog.Logger
}

// NewNotificationService создает новый экземпляр NotificationService
func NewNotificationService(repo domain.NotificationRepository, logger *slog.Logger) *NotificationService {
	return &NotificationService{
		repo:   repo,
		logger: logger,
	}
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
			var msg domain.ReadEvent
			if err := conn.ReadJSON(&msg); err != nil {
				errChan <- fmt.Errorf("ошибка чтения сообщения от клиента: %w", err)
				return
			}

			if err := s.handleReadAck(ctx, userID, login, &msg, conn); err != nil {
				s.logger.Error("Ошибка обработки ACK", "error", err)
				// Не завершаем соединение из-за ошибки ACK
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
		}
		status = domain.StatusUnread
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
		}
		status = domain.StatusAutoCleared
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
