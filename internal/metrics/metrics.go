// Package metrics регистрирует бизнес-метрики платформы (Prometheus) и отдаёт
// их по HTTP на /metrics. Один набор метрик на все сервисы; конкретный сервис
// инкрементит нужные. Стандартные Go/process-метрики идут из дефолтного реестра.
package metrics

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

var (
	// OrdersCreated — оформлено заказов, по способу получения.
	OrdersCreated = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "tea_orders_created_total",
		Help: "Количество созданных заказов по fulfillment_type",
	}, []string{"fulfillment"})

	// OrderStatus — переходы заказа в статусы (created/payment_pending/…/completed).
	OrderStatus = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "tea_order_status_total",
		Help: "Переходы заказов в статусы саги",
	}, []string{"status"})

	// CheckoutDuration — время оформления заказа (gRPC CreateOrder).
	CheckoutDuration = promauto.NewHistogram(prometheus.HistogramOpts{
		Name:    "tea_checkout_duration_seconds",
		Help:    "Длительность оформления заказа",
		Buckets: prometheus.DefBuckets,
	})

	// PaymentsTotal — результаты оплаты (succeeded/failed).
	PaymentsTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "tea_payments_total",
		Help: "Результаты обработки оплаты",
	}, []string{"status"})

	// ReservationsTotal — результаты резервирования остатков (reserved/failed).
	ReservationsTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "tea_reservations_total",
		Help: "Результаты резервирования остатков",
	}, []string{"result"})

	// Compensations — сработавшие компенсации саги (release резерва).
	Compensations = promauto.NewCounter(prometheus.CounterOpts{
		Name: "tea_saga_compensations_total",
		Help: "Количество выполненных компенсаций саги (release)",
	})

	// DeliveriesTotal — события доставки (dispatched/delivered).
	DeliveriesTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "tea_deliveries_total",
		Help: "События доставки",
	}, []string{"event"})

	// EventsConsumed — обработанные события из Kafka, по топику.
	EventsConsumed = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "tea_events_consumed_total",
		Help: "Успешно обработанные события из Kafka",
	}, []string{"topic"})

	// OutboxPublished — опубликованные relay события, по источнику.
	OutboxPublished = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "tea_outbox_published_total",
		Help: "Опубликованные из outbox события",
	}, []string{"source"})
)

// Serve запускает HTTP-эндпоинт /metrics на addr (блокирующий вызов; обычно в
// отдельной горутине).
func Serve(addr string, log *slog.Logger) {
	mux := http.NewServeMux()
	mux.Handle("/metrics", promhttp.Handler())
	srv := &http.Server{
		Addr:              addr,
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}
	log.Info("metrics endpoint запущен", "addr", addr)
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Error("metrics endpoint остановлен", "error", err)
	}
}
