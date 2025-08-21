package handler

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"notification-mvp/internal/domain"
	websocketManager "notification-mvp/internal/websocket"

	"github.com/gorilla/websocket"
)

// Handlers содержит обработчики HTTP запросов
type Handlers struct {
	service           domain.NotificationService
	repo              domain.NotificationRepository
	connectionManager *websocketManager.ConnectionManager
	logger            *slog.Logger
	upgrader          websocket.Upgrader
}

// NewHandlers создает новый экземпляр Handlers
func NewHandlers(
	service domain.NotificationService,
	repo domain.NotificationRepository,
	connectionManager *websocketManager.ConnectionManager,
	logger *slog.Logger,
) *Handlers {
	upgrader := websocket.Upgrader{
		CheckOrigin: func(r *http.Request) bool {
			// Для MVP разрешаем любые origins
			return true
		},
		ReadBufferSize:  1024,
		WriteBufferSize: 1024,
	}

	return &Handlers{
		service:           service,
		repo:              repo,
		connectionManager: connectionManager,
		logger:            logger,
		upgrader:          upgrader,
	}
}

// NotifyHandler обрабатывает POST /api/v1/notify
func (h *Handlers) NotifyHandler(w http.ResponseWriter, r *http.Request) {
	// Получаем идемпотентный ключ из заголовка
	idempotencyKey := r.Header.Get("Idempotency-Key")

	// Декодируем JSON запрос
	var req domain.NotifyRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.logger.Warn("Ошибка декодирования JSON", "error", err)
		h.writeErrorResponse(w, http.StatusBadRequest, "Неверный формат JSON")
		return
	}

	// Устанавливаем время создания если не указано
	if req.CreatedAt.IsZero() {
		req.CreatedAt = time.Now()
	}

	h.logger.Debug("Получен запрос на создание уведомлений",
		"targets_count", len(req.Target),
		"source", req.Source,
		"idempotency_key", idempotencyKey)

	// Создаем уведомления через сервис
	response, err := h.service.CreateNotifications(r.Context(), &req, idempotencyKey)
	if err != nil {
		h.logger.Error("Ошибка создания уведомлений", "error", err)
		h.writeErrorResponse(w, http.StatusInternalServerError, "Внутренняя ошибка сервера")
		return
	}

	// Возвращаем успешный ответ
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted) // 202 как указано в ТЗ

	if err := json.NewEncoder(w).Encode(response); err != nil {
		h.logger.Error("Ошибка кодирования ответа", "error", err)
	}

	h.logger.Info("Уведомления созданы успешно",
		"created_count", len(response.Results),
		"requested_count", len(req.Target))
}

// WebSocketHandler обрабатывает GET /ws
func (h *Handlers) WebSocketHandler(w http.ResponseWriter, r *http.Request) {
	// Получаем параметры пользователя из query string
	userIDStr := r.URL.Query().Get("user_id")
	login := r.URL.Query().Get("login")

	if userIDStr == "" || login == "" {
		h.logger.Warn("Отсутствуют обязательные параметры WebSocket",
			"user_id", userIDStr, "login", login)
		http.Error(w, "Требуются параметры user_id и login", http.StatusBadRequest)
		return
	}

	userID, err := strconv.ParseInt(userIDStr, 10, 64)
	if err != nil {
		h.logger.Warn("Неверный формат user_id", "user_id", userIDStr, "error", err)
		http.Error(w, "Неверный формат user_id", http.StatusBadRequest)
		return
	}

	// Апгрейдим соединение до WebSocket
	conn, err := h.upgrader.Upgrade(w, r, nil)
	if err != nil {
		h.logger.Error("Ошибка апгрейда до WebSocket", "error", err)
		return
	}
	defer func() {
		if err := conn.Close(); err != nil {
			slog.Error(err.Error(), slog.Any("error", err))
		}
	}()

	h.logger.Info("WebSocket соединение установлено", "user_id", userID, "login", login)

	// Создаем обертку для WebSocket соединения
	wsConn := &WebSocketWrapper{conn: conn}

	// Добавляем клиента в менеджер соединений
	h.connectionManager.AddClient(userID, login, wsConn)
	defer h.connectionManager.RemoveClient(userID, login)

	// Передаем соединение сервису для обработки
	if err := h.service.HandleWebSocketConnection(r.Context(), userID, login, wsConn); err != nil {
		h.logger.Error("Ошибка обработки WebSocket соединения", "error", err, "user_id", userID, "login", login)
	}

	h.logger.Info("WebSocket соединение закрыто", "user_id", userID, "login", login)
}

// HealthHandler обрабатывает GET /health
func (h *Handlers) HealthHandler(w http.ResponseWriter, r *http.Request) {
	response := map[string]interface{}{
		"status":    "ok",
		"timestamp": time.Now().Format(time.RFC3339),
		"service":   "notification-mvp",
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)

	if err := json.NewEncoder(w).Encode(response); err != nil {
		h.logger.Error("Ошибка кодирования health ответа", "error", err)
	}
}

// IndexHandler возвращает улучшенную HTML страницу для демонстрации
func (h *Handlers) IndexHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, err := w.Write([]byte(EnhancedWebUI))
	if err != nil {
		slog.Error(err.Error(), slog.Any("error", err))
	}
}

// ConnectedClientsHandler возвращает список подключенных клиентов
func (h *Handlers) ConnectedClientsHandler(w http.ResponseWriter, r *http.Request) {
	clients := h.connectionManager.GetConnectedClients()

	response := map[string]interface{}{
		"connected_clients": clients,
		"total_count":       len(clients),
		"timestamp":         time.Now().Format(time.RFC3339),
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)

	if err := json.NewEncoder(w).Encode(response); err != nil {
		h.logger.Error("Ошибка кодирования ответа connected clients", "error", err)
	}
}

// PendingNotificationsHandler возвращает список pending уведомлений
func (h *Handlers) PendingNotificationsHandler(w http.ResponseWriter, r *http.Request) {
	allPending, err := h.repo.GetAllPendingNotifications(r.Context())
	if err != nil {
		h.logger.Error("Ошибка получения pending уведомлений", "error", err)
		h.writeErrorResponse(w, http.StatusInternalServerError, "Ошибка получения pending уведомлений")
		return
	}

	// Подсчитываем статистику
	totalPending := 0
	for _, messages := range allPending {
		totalPending += len(messages)
	}

	response := map[string]interface{}{
		"pending_notifications": allPending,
		"users_with_pending":    len(allPending),
		"total_pending":         totalPending,
		"timestamp":             time.Now().Format(time.RFC3339),
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)

	if err := json.NewEncoder(w).Encode(response); err != nil {
		h.logger.Error("Ошибка кодирования ответа pending notifications", "error", err)
	}
}

// AvailableUsersHandler возвращает список пользователей доступных для отправки
func (h *Handlers) AvailableUsersHandler(w http.ResponseWriter, r *http.Request) {
	users := h.connectionManager.GetUniqueUsers()

	response := map[string]interface{}{
		"available_users": users,
		"total_count":     len(users),
		"timestamp":       time.Now().Format(time.RFC3339),
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)

	if err := json.NewEncoder(w).Encode(response); err != nil {
		h.logger.Error("Ошибка кодирования ответа available users", "error", err)
	}
}

// writeErrorResponse записывает ошибку в HTTP ответ
func (h *Handlers) writeErrorResponse(w http.ResponseWriter, statusCode int, message string) {
	errorResponse := map[string]interface{}{
		"error":     message,
		"timestamp": time.Now().Format(time.RFC3339),
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)

	if err := json.NewEncoder(w).Encode(errorResponse); err != nil {
		h.logger.Error("Ошибка кодирования error ответа", "error", err)
	}
}

// WebSocketWrapper адаптирует gorilla/websocket к нашему интерфейсу
type WebSocketWrapper struct {
	conn *websocket.Conn
}

// ReadJSON читает JSON сообщение из WebSocket
func (w *WebSocketWrapper) ReadJSON(v interface{}) error {
	return w.conn.ReadJSON(v)
}

// WriteJSON отправляет JSON сообщение в WebSocket
func (w *WebSocketWrapper) WriteJSON(v interface{}) error {
	return w.conn.WriteJSON(v)
}

// Close закрывает WebSocket соединение
func (w *WebSocketWrapper) Close() error {
	return w.conn.Close()
}
