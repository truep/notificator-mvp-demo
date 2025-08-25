package worker

import (
	"context"
	"log/slog"
	"strconv"
	"strings"
	"time"

	"notification-mvp/internal/domain"
)

// RetentionTrimmer выполняет периодический XTRIM MINID по per-user TTL
type RetentionTrimmer struct {
	repo   domain.NotificationRepository
	logger *slog.Logger
	tick   time.Duration
}

func NewRetentionTrimmer(repo domain.NotificationRepository, logger *slog.Logger) *RetentionTrimmer {
	return &RetentionTrimmer{repo: repo, logger: logger, tick: 1 * time.Minute}
}

func (w *RetentionTrimmer) Start(ctx context.Context) {
	ticker := time.NewTicker(w.tick)
	defer ticker.Stop()
	w.logger.Info("Retention trimmer запущен")
	for {
		select {
		case <-ctx.Done():
			w.logger.Info("Retention trimmer остановлен")
			return
		case <-ticker.C:
			w.runOnce(ctx)
		}
	}
}

func (w *RetentionTrimmer) runOnce(ctx context.Context) {
	// Используем существующий SCAN-список через TTL планировщик, чтобы получить userKeys
	userKeys, err := w.repo.GetAllUserKeys(ctx)
	if err != nil {
		w.logger.Warn("Ошибка получения userKeys для тримминга", "error", err)
		return
	}
	for _, uk := range userKeys {
		select {
		case <-ctx.Done():
			return
		default:
		}
		// uk в формате id-login, нужно распарсить
		// В репозитории TrimUserStreamByRetention уже знает userID/login, так что распарсим здесь схожим путем
		// Чтобы не дублировать, можно временно пропустить сложные кейсы — uk корректен
		parts := strings.SplitN(uk, "-", 2)
		if len(parts) != 2 {
			continue
		}
		userID, err := strconv.ParseInt(parts[0], 10, 64)
		if err != nil {
			continue
		}
		login := parts[1]
		if err := w.repo.TrimUserStreamByRetention(ctx, userID, login); err != nil {
			w.logger.Warn("Ошибка тримминга по retention", "user", uk, "error", err)
		}
		// лёгкий троттлинг
		time.Sleep(10 * time.Millisecond)
	}
}
