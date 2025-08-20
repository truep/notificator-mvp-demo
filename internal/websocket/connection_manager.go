package websocket

import (
	"log/slog"
	"sync"
	"time"

	"notification-mvp/internal/domain"
)

// ClientInfo содержит информацию о подключенном клиенте
type ClientInfo struct {
	UserID      int64                      `json:"user_id"`
	Login       string                     `json:"login"`
	ConnectedAt time.Time                  `json:"connected_at"`
	Connection  domain.WebSocketConnection `json:"-"`
}

// ConnectionManager управляет активными WebSocket соединениями
type ConnectionManager struct {
	clients map[string]*ClientInfo // ключ: "userID-login"
	mutex   sync.RWMutex
	logger  *slog.Logger
}

// NewConnectionManager создает новый менеджер соединений
func NewConnectionManager(logger *slog.Logger) *ConnectionManager {
	return &ConnectionManager{
		clients: make(map[string]*ClientInfo),
		logger:  logger,
	}
}

// AddClient добавляет нового клиента
func (cm *ConnectionManager) AddClient(userID int64, login string, conn domain.WebSocketConnection) {
	cm.mutex.Lock()
	defer cm.mutex.Unlock()

	key := makeClientKey(userID, login)

	// Если клиент уже подключен, закрываем старое соединение
	if existing, exists := cm.clients[key]; exists {
		cm.logger.Info("Закрытие старого соединения для клиента", "user_id", userID, "login", login)
		existing.Connection.Close()
	}

	cm.clients[key] = &ClientInfo{
		UserID:      userID,
		Login:       login,
		ConnectedAt: time.Now(),
		Connection:  conn,
	}

	cm.logger.Info("Клиент подключен", "user_id", userID, "login", login, "total_clients", len(cm.clients))
}

// RemoveClient удаляет клиента
func (cm *ConnectionManager) RemoveClient(userID int64, login string) {
	cm.mutex.Lock()
	defer cm.mutex.Unlock()

	key := makeClientKey(userID, login)
	if _, exists := cm.clients[key]; exists {
		delete(cm.clients, key)
		cm.logger.Info("Клиент отключен", "user_id", userID, "login", login, "total_clients", len(cm.clients))
	}
}

// GetConnectedClients возвращает список всех подключенных клиентов
func (cm *ConnectionManager) GetConnectedClients() []ClientInfo {
	cm.mutex.RLock()
	defer cm.mutex.RUnlock()

	clients := make([]ClientInfo, 0, len(cm.clients))
	for _, client := range cm.clients {
		// Создаем копию без connection для безопасности
		clients = append(clients, ClientInfo{
			UserID:      client.UserID,
			Login:       client.Login,
			ConnectedAt: client.ConnectedAt,
		})
	}

	return clients
}

// GetClientCount возвращает количество подключенных клиентов
func (cm *ConnectionManager) GetClientCount() int {
	cm.mutex.RLock()
	defer cm.mutex.RUnlock()
	return len(cm.clients)
}

// IsClientConnected проверяет подключен ли клиент
func (cm *ConnectionManager) IsClientConnected(userID int64, login string) bool {
	cm.mutex.RLock()
	defer cm.mutex.RUnlock()

	key := makeClientKey(userID, login)
	_, exists := cm.clients[key]
	return exists
}

// BroadcastToAll отправляет сообщение всем подключенным клиентам
func (cm *ConnectionManager) BroadcastToAll(message interface{}) {
	cm.mutex.RLock()
	clients := make([]*ClientInfo, 0, len(cm.clients))
	for _, client := range cm.clients {
		clients = append(clients, client)
	}
	cm.mutex.RUnlock()

	for _, client := range clients {
		if err := client.Connection.WriteJSON(message); err != nil {
			cm.logger.Warn("Ошибка отправки broadcast сообщения",
				"user_id", client.UserID,
				"login", client.Login,
				"error", err)
		}
	}
}

// GetUniqueUsers возвращает список уникальных пользователей (для множественной отправки)
func (cm *ConnectionManager) GetUniqueUsers() []domain.Target {
	cm.mutex.RLock()
	defer cm.mutex.RUnlock()

	uniqueUsers := make(map[string]domain.Target)
	for _, client := range cm.clients {
		key := makeClientKey(client.UserID, client.Login)
		uniqueUsers[key] = domain.Target{
			ID:    client.UserID,
			Login: client.Login,
		}
	}

	users := make([]domain.Target, 0, len(uniqueUsers))
	for _, user := range uniqueUsers {
		users = append(users, user)
	}

	return users
}

// makeClientKey создает ключ для клиента
func makeClientKey(userID int64, login string) string {
	return domain.UserKey(userID, login)
}

