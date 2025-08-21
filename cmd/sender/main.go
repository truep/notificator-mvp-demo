package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"log/slog"
	"net/http"
	"time"
)

// NotifyRequest структура для отправки уведомлений
type NotifyRequest struct {
	Target    []Target  `json:"target"`
	Message   string    `json:"message"`
	CreatedAt time.Time `json:"created_at"`
	Source    string    `json:"source"`
}

type Target struct {
	ID    int64  `json:"id"`
	Login string `json:"login"`
}

type NotifyResponse struct {
	Results []NotifyResult `json:"results"`
}

type NotifyResult struct {
	Target         Target `json:"target"`
	NotificationID string `json:"notification_id"`
}

func main() {
	var (
		addr     = flag.String("addr", "http://localhost:8080", "адрес сервера")
		userID   = flag.Int64("user", 1, "ID получателя")
		login    = flag.String("login", "test_user", "логин получателя")
		message  = flag.String("message", "Тестовое уведомление", "текст сообщения")
		source   = flag.String("source", "test-sender", "источник уведомления")
		count    = flag.Int("count", 1, "количество уведомлений для отправки")
		interval = flag.Duration("interval", time.Second, "интервал между отправками")
	)
	flag.Parse()

	fmt.Printf("Отправка %d уведомлений на %s\n", *count, *addr)
	fmt.Printf("Получатель: %d (%s)\n", *userID, *login)
	fmt.Printf("Сообщение: %s\n", *message)
	fmt.Printf("Интервал: %v\n", *interval)
	fmt.Println()

	client := &http.Client{
		Timeout: 10 * time.Second,
	}

	for i := 0; i < *count; i++ {
		messageText := *message
		if *count > 1 {
			messageText = fmt.Sprintf("%s #%d", *message, i+1)
		}

		err := sendNotification(client, *addr, *userID, *login, messageText, *source)
		if err != nil {
			log.Printf("Ошибка отправки уведомления %d: %v", i+1, err)
		} else {
			fmt.Printf("✅ Уведомление %d/%d отправлено\n", i+1, *count)
		}

		if i < *count-1 {
			time.Sleep(*interval)
		}
	}

	fmt.Println("\nВсе уведомления отправлены!")
}

func sendNotification(client *http.Client, addr string, userID int64, login, message, source string) error {
	req := NotifyRequest{
		Target: []Target{
			{
				ID:    userID,
				Login: login,
			},
		},
		Message:   message,
		CreatedAt: time.Now(),
		Source:    source,
	}

	jsonData, err := json.Marshal(req)
	if err != nil {
		return fmt.Errorf("ошибка сериализации JSON: %w", err)
	}

	url := addr + "/api/v1/notify"
	resp, err := client.Post(url, "application/json", bytes.NewBuffer(jsonData))
	if err != nil {
		return fmt.Errorf("ошибка HTTP запроса: %w", err)
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			slog.Error(err.Error(), slog.Any("error", err))
		}
	}()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("ошибка чтения ответа: %w", err)
	}

	if resp.StatusCode != http.StatusAccepted {
		return fmt.Errorf("неожиданный статус %d: %s", resp.StatusCode, string(body))
	}

	var response NotifyResponse
	if err := json.Unmarshal(body, &response); err != nil {
		return fmt.Errorf("ошибка парсинга ответа: %w", err)
	}

	if len(response.Results) == 0 {
		return fmt.Errorf("нет результатов в ответе")
	}

	fmt.Printf("  Notification ID: %s\n", response.Results[0].NotificationID)
	return nil
}
