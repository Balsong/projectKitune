package inventory

import (
	"context"
	"log/slog"

	"github.com/jackc/pgx/v5/pgxpool"

	"tea-platform/internal/outbox"
	"tea-platform/pkg/events"
)

// Service обрабатывает order.created: резервирует остатки и эмитит результат.
type Service struct {
	pool   *pgxpool.Pool
	repo   *Repository
	outbox *outbox.Repo
	log    *slog.Logger
}

// NewService собирает сервис инвентаря.
func NewService(pool *pgxpool.Pool, outboxRepo *outbox.Repo, log *slog.Logger) *Service {
	return &Service{pool: pool, repo: NewRepository(pool), outbox: outboxRepo, log: log}
}

// --- payloads ---

type orderItem struct {
	ProductID string `json:"product_id"`
	Quantity  int32  `json:"quantity"`
}

type orderCreatedPayload struct {
	OrderID string      `json:"order_id"`
	Items   []orderItem `json:"items"`
}

type reservedItem struct {
	ProductID string `json:"product_id"`
	Quantity  int32  `json:"quantity"`
}

type stockReservedPayload struct {
	OrderID  string         `json:"order_id"`
	Reserved []reservedItem `json:"reserved"`
}

type reservationFailedPayload struct {
	OrderID          string   `json:"order_id"`
	Reason           string   `json:"reason"`
	UnavailableItems []string `json:"unavailable_items"`
}

// HandleOrderEvent диспетчеризует события заказа: created → резерв,
// confirmed → commit резерва, cancelled → release (компенсация).
func (s *Service) HandleOrderEvent(ctx context.Context, env events.Envelope) error {
	switch env.EventType {
	case events.EventOrderCreated:
		return s.reserve(ctx, env)
	case events.EventOrderConfirmed:
		return s.commit(ctx, env)
	case events.EventOrderCancelled:
		return s.release(ctx, env)
	}
	return nil
}

// commit фиксирует резервы оплаченного заказа.
func (s *Service) commit(ctx context.Context, env events.Envelope) error {
	var p struct {
		OrderID string `json:"order_id"`
	}
	if err := env.UnmarshalPayload(&p); err != nil {
		return err
	}
	n, err := s.repo.Commit(ctx, p.OrderID)
	if err != nil {
		return err
	}
	if n > 0 {
		s.log.Info("резерв подтверждён (committed)", "order_id", p.OrderID, "items", n)
	}
	return nil
}

// release освобождает резервы отменённого заказа (компенсация саги).
func (s *Service) release(ctx context.Context, env events.Envelope) error {
	var p struct {
		OrderID string `json:"order_id"`
	}
	if err := env.UnmarshalPayload(&p); err != nil {
		return err
	}
	n, err := s.repo.Release(ctx, p.OrderID)
	if err != nil {
		return err
	}
	if n > 0 {
		s.log.Info("резерв освобождён (released)", "order_id", p.OrderID, "items", n)
	}
	return nil
}

// reserve резервирует остатки под заказ. Успех (stock.reserved) пишется в одной
// транзакции с резервами; отказ (reservation.failed) — отдельно, так как резерв
// откатывается. correlation_id протягивается из order.created.
func (s *Service) reserve(ctx context.Context, env events.Envelope) error {
	var p orderCreatedPayload
	if err := env.UnmarshalPayload(&p); err != nil {
		return err
	}

	// Идемпотентность: уже резервировали под этот заказ — выходим.
	exists, err := s.repo.HasReservations(ctx, p.OrderID)
	if err != nil {
		return err
	}
	if exists {
		s.log.Info("резерв под заказ уже есть, пропускаю", "order_id", p.OrderID)
		return nil
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback(ctx)
		}
	}()

	var failed []string
	reserved := make([]reservedItem, 0, len(p.Items))
	for _, it := range p.Items {
		ok, rerr := reserveItem(ctx, tx, p.OrderID, it.ProductID, it.Quantity)
		if rerr != nil {
			return rerr // инфраструктурная ошибка → повтор сообщения
		}
		if !ok {
			failed = append(failed, it.ProductID)
			continue
		}
		reserved = append(reserved, reservedItem{ProductID: it.ProductID, Quantity: it.Quantity})
	}

	// Отказ: откатываем резерв и эмитим reservation.failed отдельной транзакцией.
	if len(failed) > 0 {
		_ = tx.Rollback(ctx)
		failEnv, ferr := events.NewFollow(events.EventReservationFailed, 1, p.OrderID,
			reservationFailedPayload{OrderID: p.OrderID, Reason: "out_of_stock", UnavailableItems: failed}, env)
		if ferr != nil {
			return ferr
		}
		if err := s.outbox.Save(ctx, outbox.SourceInventory, events.TopicInventory, failEnv); err != nil {
			return err
		}
		s.log.Info("резерв не удался", "order_id", p.OrderID, "unavailable", failed)
		return nil
	}

	// Успех: stock.reserved в той же транзакции, что и резервы.
	okEnv, oerr := events.NewFollow(events.EventStockReserved, 1, p.OrderID,
		stockReservedPayload{OrderID: p.OrderID, Reserved: reserved}, env)
	if oerr != nil {
		return oerr
	}
	if err := outbox.SaveTx(ctx, tx, outbox.SourceInventory, events.TopicInventory, okEnv); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return err
	}
	committed = true

	s.log.Info("остатки зарезервированы", "order_id", p.OrderID, "items", len(reserved))
	return nil
}
