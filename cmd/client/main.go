package main

import (
	"bufio"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strings"
	"time"

	"github.com/gorilla/websocket"
)

// WebSocket сообщения
type PushMessage struct {
	Type string      `json:"type"`
	Data PushPayload `json:"data"`
}

type PushPayload struct {
	NotificationID string    `json:"notification_id"`
	StreamID       string    `json:"stream_id"`
	Message        string    `json:"message,omitempty"`
	CreatedAt      time.Time `json:"created_at"`
	Source         string    `json:"source"`
	Status         string    `json:"status"`
}

type ReadEvent struct {
	Type string   `json:"type"`
	Data ReadData `json:"data"`
}

type ReadData struct {
	NotificationID string `json:"notification_id"`
	StreamID       string `json:"stream_id"`
}

func main() {
	var (
		addr   = flag.String("addr", "localhost:8080", "адрес сервера")
		userID = flag.Int64("user", 1, "ID пользователя")
		login  = flag.String("login", "test_user", "логин пользователя")
		auto   = flag.Bool("auto", false, "автоматически подтверждать уведомления")
	)
	flag.Parse()

	// Подключаемся к WebSocket
	url := fmt.Sprintf("ws://%s/ws?user_id=%d&login=%s", *addr, *userID, *login)
	fmt.Printf("Подключение к %s\n", url)

	conn, _, err := websocket.DefaultDialer.Dial(url, nil)
	if err != nil {
		log.Fatal("Ошибка подключения:", err)
	}
	defer conn.Close()

	fmt.Printf("Подключен как пользователь %d (%s)\n", *userID, *login)
	fmt.Println("Команды:")
	fmt.Println("  help - показать справку")
	fmt.Println("  ack <notification_id> <stream_id> - подтвердить уведомление")
	fmt.Println("  quit - выйти")
	fmt.Println()

	// Канал для завершения
	interrupt := make(chan os.Signal, 1)
	signal.Notify(interrupt, os.Interrupt)

	// Канал для команд пользователя
	commands := make(chan string)

	// Горутина для чтения ввода пользователя
	go func() {
		scanner := bufio.NewScanner(os.Stdin)
		for scanner.Scan() {
			commands <- scanner.Text()
		}
	}()

	// Горутина для чтения сообщений от сервера
	go func() {
		for {
			var msg PushMessage
			err := conn.ReadJSON(&msg)
			if err != nil {
				if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
					log.Printf("Ошибка чтения WebSocket: %v", err)
				}
				return
			}

			handleMessage(&msg, conn, *auto)
		}
	}()

	// Основной цикл
	for {
		select {
		case <-interrupt:
			fmt.Println("\nЗавершение...")
			conn.WriteMessage(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""))
			return

		case cmd := <-commands:
			if !handleCommand(cmd, conn) {
				return
			}
		}
	}
}

func handleMessage(msg *PushMessage, conn *websocket.Conn, autoAck bool) {
	switch msg.Type {
	case "notification.push":
		if msg.Data.Status == "unread" {
			fmt.Printf("\n🔔 Новое уведомление:\n")
			fmt.Printf("  ID: %s\n", msg.Data.NotificationID)
			fmt.Printf("  Источник: %s\n", msg.Data.Source)
			fmt.Printf("  Сообщение: %s\n", msg.Data.Message)
			fmt.Printf("  Время: %s\n", msg.Data.CreatedAt.Format("15:04:05"))
			fmt.Printf("  Stream ID: %s\n", msg.Data.StreamID)

			if autoAck {
				// Автоматически подтверждаем через секунду
				time.Sleep(1 * time.Second)
				sendAck(conn, msg.Data.NotificationID, msg.Data.StreamID)
				fmt.Printf("  ✅ Автоматически подтверждено\n")
			} else {
				fmt.Printf("  Для подтверждения: ack %s %s\n", msg.Data.NotificationID, msg.Data.StreamID)
			}
		} else if msg.Data.Status == "auto_cleared" {
			fmt.Printf("\n🗑️ Просроченное уведомление (автоматически удалено):\n")
			fmt.Printf("  ID: %s\n", msg.Data.NotificationID)
			fmt.Printf("  Stream ID: %s\n", msg.Data.StreamID)
		}

	case "notification.read.ack":
		fmt.Printf("\n✅ Подтверждение получено от сервера\n")

	case "error":
		fmt.Printf("\n❌ Ошибка от сервера: %+v\n", msg.Data)

	default:
		fmt.Printf("\n📨 Неизвестное сообщение: %+v\n", msg)
	}

	fmt.Print("> ")
}

func handleCommand(cmd string, conn *websocket.Conn) bool {
	cmd = strings.TrimSpace(cmd)
	if cmd == "" {
		return true
	}

	parts := strings.Fields(cmd)
	switch parts[0] {
	case "help":
		fmt.Println("Команды:")
		fmt.Println("  help - показать справку")
		fmt.Println("  ack <notification_id> <stream_id> - подтвердить уведомление")
		fmt.Println("  quit - выйти")

	case "ack":
		if len(parts) != 3 {
			fmt.Println("Использование: ack <notification_id> <stream_id>")
			return true
		}
		sendAck(conn, parts[1], parts[2])
		fmt.Println("Отправлено подтверждение")

	case "quit", "exit":
		return false

	default:
		fmt.Printf("Неизвестная команда: %s (введите 'help' для справки)\n", parts[0])
	}

	return true
}

func sendAck(conn *websocket.Conn, notificationID, streamID string) {
	ackMsg := ReadEvent{
		Type: "notification.read",
		Data: ReadData{
			NotificationID: notificationID,
			StreamID:       streamID,
		},
	}

	if err := conn.WriteJSON(ackMsg); err != nil {
		log.Printf("Ошибка отправки ACK: %v", err)
	}
}
