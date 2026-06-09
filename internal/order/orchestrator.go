package order

import (
	"context"
	"log/slog"

	"tea-platform/internal/outbox"
	"tea-platform/pkg/events"
)

// Orchestrator — дирижёр саги оформления заказа. Реагирует на события инвентаря
// и оплаты, продвигает state machine заказа и эмитит order.confirmed /
// order.cancelled (последнее запускает компенсацию — release резерва).
type Orchestrator struct {
	repo *Repository
	log  *slog.Logger
}

// NewOrchestrator собирает дирижёра.
func NewOrchestrator(repo *Repository, log *slog.Logger) *Orchestrator {
	return &Orchestrator{repo: repo, log: log}
}

type orderRef struct {
	OrderID string `json:"order_id"`
}

type orderCancelledPayload struct {
	OrderID string `json:"order_id"`
	Reason  string `json:"reason"`
}

// HandleInventoryEvent: stock.reserved → payment_pending; reservation.failed →
// cancelled (+ order.cancelled).
func (o *Orchestrator) HandleInventoryEvent(ctx context.Context, env events.Envelope) error {
	var p orderRef
	if err := env.UnmarshalPayload(&p); err != nil {
		return err
	}

	switch env.EventType {
	case events.EventStockReserved:
		applied, err := o.repo.Transition(ctx, p.OrderID, StatusCreated, StatusPaymentPending, nil)
		if err != nil {
			return err
		}
		if applied {
			o.log.Info("заказ → payment_pending", "order_id", p.OrderID)
		}
		return nil

	case events.EventReservationFailed:
		cancelEnv, err := events.NewFollow(events.EventOrderCancelled, 1, p.OrderID,
			orderCancelledPayload{OrderID: p.OrderID, Reason: "reservation_failed"}, env)
		if err != nil {
			return err
		}
		applied, err := o.repo.Transition(ctx, p.OrderID, StatusCreated, StatusCancelled,
			&EmitSpec{Source: outbox.SourceOrder, Topic: events.TopicOrders, Env: cancelEnv})
		if err != nil {
			return err
		}
		if applied {
			o.log.Info("заказ → cancelled (нет резерва)", "order_id", p.OrderID)
		}
		return nil
	}
	return nil
}

// HandlePaymentEvent: payment.succeeded → confirmed (+ order.confirmed);
// payment.failed → payment_failed (+ order.cancelled, запуск компенсации).
func (o *Orchestrator) HandlePaymentEvent(ctx context.Context, env events.Envelope) error {
	var p orderRef
	if err := env.UnmarshalPayload(&p); err != nil {
		return err
	}

	switch env.EventType {
	case events.EventPaymentSucceeded:
		confirmEnv, err := events.NewFollow(events.EventOrderConfirmed, 1, p.OrderID,
			orderRef{OrderID: p.OrderID}, env)
		if err != nil {
			return err
		}
		applied, err := o.repo.Transition(ctx, p.OrderID, StatusPaymentPending, StatusConfirmed,
			&EmitSpec{Source: outbox.SourceOrder, Topic: events.TopicOrders, Env: confirmEnv})
		if err != nil {
			return err
		}
		if applied {
			o.log.Info("заказ → confirmed (оплачен)", "order_id", p.OrderID)
		}
		return nil

	case events.EventPaymentFailed:
		cancelEnv, err := events.NewFollow(events.EventOrderCancelled, 1, p.OrderID,
			orderCancelledPayload{OrderID: p.OrderID, Reason: "payment_failed"}, env)
		if err != nil {
			return err
		}
		applied, err := o.repo.Transition(ctx, p.OrderID, StatusPaymentPending, StatusPaymentFailed,
			&EmitSpec{Source: outbox.SourceOrder, Topic: events.TopicOrders, Env: cancelEnv})
		if err != nil {
			return err
		}
		if applied {
			o.log.Info("заказ → payment_failed (запуск компенсации)", "order_id", p.OrderID)
		}
		return nil
	}
	return nil
}
