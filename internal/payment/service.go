// Package payment реализует mock платёжного провайдера (ЮKassa). Слушает
// stock.reserved, берёт сумму заказа из Order по gRPC, «проводит» оплату и
// эмитит payment.succeeded / payment.failed.
package payment

import (
	"context"
	"log/slog"

	"github.com/jackc/pgx/v5/pgxpool"

	orderv1 "tea-platform/internal/genpb/order/v1"
	"tea-platform/internal/outbox"
	"tea-platform/pkg/events"
)

// Service обрабатывает stock.reserved и инициирует оплату.
type Service struct {
	pool       *pgxpool.Pool
	outbox     *outbox.Repo
	order      orderv1.OrderServiceClient
	limitCents int64
	log        *slog.Logger
}

// NewService собирает сервис оплаты. limitCents — порог отказа («лимит карты»).
func NewService(pool *pgxpool.Pool, outboxRepo *outbox.Repo, order orderv1.OrderServiceClient, limitCents int64, log *slog.Logger) *Service {
	return &Service{pool: pool, outbox: outboxRepo, order: order, limitCents: limitCents, log: log}
}

type stockReservedPayload struct {
	OrderID string `json:"order_id"`
}

type paymentSucceededPayload struct {
	OrderID     string `json:"order_id"`
	AmountCents int64  `json:"amount_cents"`
	Currency    string `json:"currency"`
}

type paymentFailedPayload struct {
	OrderID     string `json:"order_id"`
	AmountCents int64  `json:"amount_cents"`
	Reason      string `json:"reason"`
}

// HandleStockReserved инициирует оплату заказа после успешного резерва.
// Сумму берёт из Order; решение об успехе — детерминированное (mock): оплата
// проходит, если сумма не превышает лимит. Платёж и событие пишутся в одной
// транзакции (outbox). Идемпотентность — по PK payments.order_id.
func (s *Service) HandleStockReserved(ctx context.Context, env events.Envelope) error {
	if env.EventType != events.EventStockReserved {
		return nil
	}

	var p stockReservedPayload
	if err := env.UnmarshalPayload(&p); err != nil {
		return err
	}

	// Идемпотентность: платёж по заказу уже есть — выходим.
	var exists bool
	if err := s.pool.QueryRow(ctx,
		`SELECT EXISTS(SELECT 1 FROM payments WHERE order_id = $1)`, p.OrderID,
	).Scan(&exists); err != nil {
		return err
	}
	if exists {
		s.log.Info("платёж по заказу уже есть, пропускаю", "order_id", p.OrderID)
		return nil
	}

	// Сумма заказа — из Order Service.
	ord, err := s.order.GetOrder(ctx, &orderv1.GetOrderRequest{Id: p.OrderID})
	if err != nil {
		return err // инфраструктурная ошибка → повтор сообщения
	}
	amount := ord.GetTotalCents()
	currency := ord.GetCurrency()

	// Mock-решение: успех, если в пределах лимита.
	success := amount <= s.limitCents

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	status := "failed"
	if success {
		status = "succeeded"
	}
	if _, err := tx.Exec(ctx,
		`INSERT INTO payments (order_id, amount_cents, currency, status)
		 VALUES ($1, $2, $3, $4) ON CONFLICT (order_id) DO NOTHING`,
		p.OrderID, amount, currency, status,
	); err != nil {
		return err
	}

	var outEnv events.Envelope
	if success {
		outEnv, err = events.NewFollow(events.EventPaymentSucceeded, 1, p.OrderID,
			paymentSucceededPayload{OrderID: p.OrderID, AmountCents: amount, Currency: currency}, env)
	} else {
		outEnv, err = events.NewFollow(events.EventPaymentFailed, 1, p.OrderID,
			paymentFailedPayload{OrderID: p.OrderID, AmountCents: amount, Reason: "card_limit_exceeded"}, env)
	}
	if err != nil {
		return err
	}
	if err := outbox.SaveTx(ctx, tx, outbox.SourcePayment, events.TopicPayments, outEnv); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return err
	}

	s.log.Info("оплата обработана",
		"order_id", p.OrderID, "amount_cents", amount, "status", status)
	return nil
}
