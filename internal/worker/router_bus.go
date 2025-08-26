package worker

import (
	"context"
	"encoding/json"
	"log/slog"
	"time"

	"github.com/redis/go-redis/v9"
)

// BusMessage структура сообщения шины
type BusMessage struct {
	Type    string          `json:"type"`
	UserKey string          `json:"userKey"`
	Data    json.RawMessage `json:"data"`
}

// InterPodRouter читает notif:bus:<podID> и доставляет локальным WS
type InterPodRouter struct {
	rdb    *redis.Client
	podID  string
	logger *slog.Logger
	// простейший коллбек-доставщик
	deliver func(userKey string, payload json.RawMessage) bool
}

func NewInterPodRouter(rdb *redis.Client, podID string, logger *slog.Logger, deliver func(userKey string, payload json.RawMessage) bool) *InterPodRouter {
	return &InterPodRouter{rdb: rdb, podID: podID, logger: logger, deliver: deliver}
}

func (r *InterPodRouter) Start(ctx context.Context) {
	group := "router"
	stream := "notif:bus:" + r.podID
	consumer := "consumer:" + r.podID

	// ensure group
	_ = r.rdb.XGroupCreateMkStream(ctx, stream, group, "$").Err()

	r.logger.Info("InterPodRouter запущен", "stream", stream)
	for {
		select {
		case <-ctx.Done():
			r.logger.Info("InterPodRouter остановлен")
			return
		default:
		}

		streams, err := r.rdb.XReadGroup(ctx, &redis.XReadGroupArgs{
			Group:    group,
			Consumer: consumer,
			Streams:  []string{stream, ">"},
			Count:    100,
			Block:    30 * time.Second,
		}).Result()
		if err != nil {
			if err == redis.Nil {
				continue
			}
			r.logger.Warn("Ошибка XREADGROUP в роутере", "error", err)
			time.Sleep(200 * time.Millisecond)
			continue
		}
		for _, s := range streams {
			for _, m := range s.Messages {
				var busMsg BusMessage
				if b, ok := m.Values["payload"].(string); ok {
					_ = json.Unmarshal([]byte(b), &busMsg)
				}
				userKey := busMsg.UserKey
				delivered := false
				if len(busMsg.Data) > 0 && userKey != "" && r.deliver != nil {
					delivered = r.deliver(userKey, busMsg.Data)
				}
				if delivered {
					_ = r.rdb.XAck(ctx, stream, group, m.ID).Err()
				}
			}
		}
	}
}
