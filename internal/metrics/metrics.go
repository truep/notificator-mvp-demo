package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
)

var (
	WSConnections = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "notif_ws_connections",
		Help: "Текущее число активных WebSocket соединений",
	})

	NotificationsSent = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "notif_messages_sent_total",
		Help: "Количество отправленных уведомлений (server->client)",
	})

	NotificationsAcked = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "notif_messages_acked_total",
		Help: "Количество подтверждений уведомлений (client->server)",
	})

	NotificationsAutoCleared = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "notif_messages_autocleared_total",
		Help: "Количество истёкших/автоочищенных уведомлений, отправленных клиенту",
	})

	DeliveryLatencyMs = prometheus.NewHistogram(prometheus.HistogramOpts{
		Name:    "notif_delivery_latency_ms",
		Help:    "Задержка доставки от времени создания до отправки клиенту (мс)",
		Buckets: []float64{10, 25, 50, 75, 100, 150, 250, 500, 1000, 2000, 5000},
	})

	ReclaimedMessages = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "notif_xautoclaim_reclaimed_total",
		Help: "Количество сообщений, перехваченных XAUTOCLAIM",
	})

	BusDelivered = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "notif_bus_delivered_total",
		Help: "Количество сообщений, доставленных через межподовую шину",
	})

	TTLCleaned = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "notif_ttl_cleaned_total",
		Help: "Количество записей, удалённых TTL-джанитором",
	})
)

func init() {
	prometheus.MustRegister(
		WSConnections,
		NotificationsSent,
		NotificationsAcked,
		NotificationsAutoCleared,
		DeliveryLatencyMs,
		ReclaimedMessages,
		BusDelivered,
		TTLCleaned,
	)
}
