// Command notification-svc — stateless-консьюмер событий платформы. Слушает
// bookings.events и orders.events и «отправляет» уведомления (пока — лог).
package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"sync"
	"syscall"

	"tea-platform/internal/config"
	"tea-platform/internal/kafka"
	"tea-platform/pkg/events"
)

func main() {
	log := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	cfg := config.Load()

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	// Каждый топик — отдельный консьюмер в своей горутине. Группа уникальна
	// на топик: один group id на разные топики ломает распределение партиций.
	subscriptions := []struct {
		topic   string
		group   string
		handler kafka.HandlerFunc
	}{
		{events.TopicBookings, "notification-svc-bookings", handleBooking(log)},
		{events.TopicOrders, "notification-svc-orders", handleOrder(log)},
	}

	var wg sync.WaitGroup
	for _, sub := range subscriptions {
		consumer := kafka.NewConsumer(cfg.KafkaBrokers, sub.topic, sub.group, log)
		wg.Add(1)
		go func(topic string, h kafka.HandlerFunc) {
			defer wg.Done()
			defer func() { _ = consumer.Close() }()
			log.Info("подписка запущена", "topic", topic)
			if err := consumer.Run(ctx, h); err != nil {
				log.Error("консьюмер остановлен с ошибкой", "topic", topic, "error", err)
			}
		}(sub.topic, sub.handler)
	}

	wg.Wait()
	log.Info("notification-svc остановлен")
}

// handleBooking «отправляет» уведомление о брони.
func handleBooking(log *slog.Logger) kafka.HandlerFunc {
	return func(_ context.Context, env events.Envelope) error {
		var p struct {
			BookingID string `json:"booking_id"`
			TableID   int    `json:"table_id"`
			Customer  string `json:"customer"`
			Time      string `json:"time"`
		}
		if err := env.UnmarshalPayload(&p); err != nil {
			return err
		}
		log.Info("📨 уведомление: бронь принята",
			"event_type", env.EventType,
			"booking_id", p.BookingID,
			"customer", p.Customer,
			"table_id", p.TableID,
			"time", p.Time,
			"correlation_id", env.CorrelationID,
		)
		return nil
	}
}

// handleOrder «отправляет» уведомление о созданном заказе.
func handleOrder(log *slog.Logger) kafka.HandlerFunc {
	return func(_ context.Context, env events.Envelope) error {
		var p struct {
			OrderID         string `json:"order_id"`
			FulfillmentType string `json:"fulfillment_type"`
			TotalCents      int64  `json:"total_cents"`
			Currency        string `json:"currency"`
		}
		if err := env.UnmarshalPayload(&p); err != nil {
			return err
		}
		log.Info("📨 уведомление: заказ принят",
			"event_type", env.EventType,
			"order_id", p.OrderID,
			"fulfillment", p.FulfillmentType,
			"total_cents", p.TotalCents,
			"currency", p.Currency,
			"correlation_id", env.CorrelationID,
		)
		return nil
	}
}
