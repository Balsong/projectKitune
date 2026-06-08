// Command notification-svc — stateless-консьюмер событий платформы. На Этапе 3a
// слушает bookings.events и «отправляет» уведомление (пока — структурный лог).
package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
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

	consumer := kafka.NewConsumer(cfg.KafkaBrokers, events.TopicBookings, "notification-svc", log)
	defer func() { _ = consumer.Close() }()

	log.Info("notification-svc запущен", "topic", events.TopicBookings)
	if err := consumer.Run(ctx, handleBooking(log)); err != nil {
		log.Error("консьюмер остановлен с ошибкой", "error", err)
		os.Exit(1)
	}
}

// handleBooking «отправляет» уведомление о брони. Реальные каналы (email/SMS/push)
// подключим позже; сейчас фиксируем факт доставки структурным логом.
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
