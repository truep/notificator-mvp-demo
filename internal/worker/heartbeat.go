package worker

import (
	"context"
	"log/slog"
	"strconv"
	"time"

	"github.com/redis/go-redis/v9"
)

// HeartbeatWorker periodically updates pod heartbeat and cleans stale entries.
type HeartbeatWorker struct {
	rdb              *redis.Client
	podID            string
	logger           *slog.Logger
	tickInterval     time.Duration
	staleAfter       time.Duration
	podsHeartbeatKey string
}

func NewHeartbeatWorker(rdb *redis.Client, podID string, logger *slog.Logger) *HeartbeatWorker {
	return &HeartbeatWorker{
		rdb:              rdb,
		podID:            podID,
		logger:           logger,
		tickInterval:     30 * time.Second,
		staleAfter:       90 * time.Second,
		podsHeartbeatKey: "notif:pods:hb",
	}
}

func (w *HeartbeatWorker) Start(ctx context.Context) {
	ticker := time.NewTicker(w.tickInterval)
	defer ticker.Stop()

	w.logger.Info("Heartbeat воркер запущен", "pod", w.podID)

	for {
		select {
		case <-ctx.Done():
			w.logger.Info("Heartbeat воркер остановлен")
			return
		case <-ticker.C:
			w.beat(ctx)
			w.cleanupStale(ctx)
		}
	}
}

func (w *HeartbeatWorker) beat(ctx context.Context) {
	now := time.Now().Unix()
	if err := w.rdb.HSet(ctx, w.podsHeartbeatKey, w.podID, strconv.FormatInt(now, 10)).Err(); err != nil {
		w.logger.Warn("Ошибка heartbeat", "error", err)
		return
	}
	w.logger.Debug("Heartbeat", "pod", w.podID, "ts", now)
}

func (w *HeartbeatWorker) cleanupStale(ctx context.Context) {
	entries, err := w.rdb.HGetAll(ctx, w.podsHeartbeatKey).Result()
	if err != nil {
		w.logger.Warn("Ошибка чтения heartbeat реестра", "error", err)
		return
	}

	if len(entries) == 0 {
		return
	}

	cutoff := time.Now().Add(-w.staleAfter).Unix()
	var stalePods []string
	for pod, tsStr := range entries {
		ts, err := strconv.ParseInt(tsStr, 10, 64)
		if err != nil {
			// Некорректные записи удаляем
			stalePods = append(stalePods, pod)
			continue
		}
		if ts < cutoff {
			stalePods = append(stalePods, pod)
		}
	}

	if len(stalePods) == 0 {
		return
	}

	if err := w.rdb.HDel(ctx, w.podsHeartbeatKey, stalePods...).Err(); err != nil {
		w.logger.Warn("Ошибка удаления протухших podов", "error", err, "pods", stalePods)
		return
	}
	w.logger.Info("Очищены протухшие pod записи", "count", len(stalePods))
}
