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

	// Гарантируем существование топиков до запуска консьюмеров (иначе гонка
	// с авто-созданием топика на стороне продюсера).
	if err := kafka.EnsureTopics(ctx, cfg.KafkaBrokers,
		events.TopicBookings, events.TopicOrders, events.TopicInventory, events.TopicPayments, events.TopicDelivery); err != nil {
		log.Error("не удалось создать топики", "error", err)
		os.Exit(1)
	}

	// Каждый топик — отдельный консьюмер в своей горутине. Группа уникальна
	// на топик: один group id на разные топики ломает распределение партиций.
	subscriptions := []struct {
		topic   string
		group   string
		handler kafka.HandlerFunc
	}{
		{events.TopicBookings, "notification-svc-bookings", handleBooking(log)},
		{events.TopicOrders, "notification-svc-orders", handleOrder(log)},
		{events.TopicInventory, "notification-svc-inventory", handleInventory(log)},
		{events.TopicPayments, "notification-svc-payments", handlePayment(log)},
		{events.TopicDelivery, "notification-svc-delivery", handleDelivery(log)},
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

// handleOrder уведомляет о жизненном цикле заказа: создан / подтверждён / отменён.
func handleOrder(log *slog.Logger) kafka.HandlerFunc {
	return func(_ context.Context, env events.Envelope) error {
		var p struct {
			OrderID         string `json:"order_id"`
			FulfillmentType string `json:"fulfillment_type"`
			TotalCents      int64  `json:"total_cents"`
			Currency        string `json:"currency"`
			Reason          string `json:"reason"`
		}
		if err := env.UnmarshalPayload(&p); err != nil {
			return err
		}
		switch env.EventType {
		case events.EventOrderCreated:
			log.Info("📨 уведомление: заказ принят",
				"order_id", p.OrderID, "fulfillment", p.FulfillmentType,
				"total_cents", p.TotalCents, "correlation_id", env.CorrelationID)
		case events.EventOrderConfirmed:
			log.Info("📨 уведомление: заказ подтверждён ✅",
				"order_id", p.OrderID, "correlation_id", env.CorrelationID)
		case events.EventOrderCancelled:
			log.Warn("📨 уведомление: заказ отменён ❌",
				"order_id", p.OrderID, "reason", p.Reason, "correlation_id", env.CorrelationID)
		}
		return nil
	}
}

// handleDelivery уведомляет о ходе доставки.
func handleDelivery(log *slog.Logger) kafka.HandlerFunc {
	return func(_ context.Context, env events.Envelope) error {
		var p struct {
			OrderID      string `json:"order_id"`
			TrackingCode string `json:"tracking_code"`
			Courier      string `json:"courier"`
			ETA          string `json:"eta"`
		}
		if err := env.UnmarshalPayload(&p); err != nil {
			return err
		}
		switch env.EventType {
		case events.EventDeliveryDispatched:
			log.Info("📨 уведомление: заказ отправлен 🚚",
				"order_id", p.OrderID, "tracking", p.TrackingCode,
				"courier", p.Courier, "eta", p.ETA, "correlation_id", env.CorrelationID)
		case events.EventDeliveryDelivered:
			log.Info("📨 уведомление: заказ доставлен 📦",
				"order_id", p.OrderID, "correlation_id", env.CorrelationID)
		}
		return nil
	}
}

// handlePayment уведомляет о результате оплаты.
func handlePayment(log *slog.Logger) kafka.HandlerFunc {
	return func(_ context.Context, env events.Envelope) error {
		var p struct {
			OrderID     string `json:"order_id"`
			AmountCents int64  `json:"amount_cents"`
			Reason      string `json:"reason"`
		}
		if err := env.UnmarshalPayload(&p); err != nil {
			return err
		}
		switch env.EventType {
		case events.EventPaymentSucceeded:
			log.Info("📨 уведомление: оплата прошла 💳",
				"order_id", p.OrderID, "amount_cents", p.AmountCents,
				"correlation_id", env.CorrelationID)
		case events.EventPaymentFailed:
			log.Warn("📨 уведомление: оплата отклонена 💳",
				"order_id", p.OrderID, "amount_cents", p.AmountCents,
				"reason", p.Reason, "correlation_id", env.CorrelationID)
		}
		return nil
	}
}

// handleInventory логирует результат резервирования остатков под заказ.
func handleInventory(log *slog.Logger) kafka.HandlerFunc {
	return func(_ context.Context, env events.Envelope) error {
		var p struct {
			OrderID          string   `json:"order_id"`
			Reason           string   `json:"reason"`
			UnavailableItems []string `json:"unavailable_items"`
		}
		if err := env.UnmarshalPayload(&p); err != nil {
			return err
		}
		switch env.EventType {
		case events.EventStockReserved:
			log.Info("📨 уведомление: остатки зарезервированы",
				"event_type", env.EventType, "order_id", p.OrderID,
				"correlation_id", env.CorrelationID)
		case events.EventReservationFailed:
			log.Warn("📨 уведомление: резерв не удался",
				"event_type", env.EventType, "order_id", p.OrderID,
				"reason", p.Reason, "unavailable", p.UnavailableItems,
				"correlation_id", env.CorrelationID)
		}
		return nil
	}
}
