package delivery

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/jackc/pgx/v5/pgxpool"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	deliveryv1 "tea-platform/internal/genpb/delivery/v1"
	orderv1 "tea-platform/internal/genpb/order/v1"
	"tea-platform/internal/metrics"
	"tea-platform/internal/order"
	"tea-platform/internal/outbox"
	"tea-platform/pkg/events"
)

// Service — gRPC-сервер доставок + событийная логика.
type Service struct {
	deliveryv1.UnimplementedDeliveryServiceServer

	pool   *pgxpool.Pool
	repo   *Repository
	outbox *outbox.Repo
	order  orderv1.OrderServiceClient
	log    *slog.Logger
}

// NewService собирает сервис доставки.
func NewService(pool *pgxpool.Pool, outboxRepo *outbox.Repo, orderClient orderv1.OrderServiceClient, log *slog.Logger) *Service {
	return &Service{pool: pool, repo: NewRepository(pool), outbox: outboxRepo, order: orderClient, log: log}
}

// GetDelivery (gRPC) возвращает статус доставки по заказу.
func (s *Service) GetDelivery(ctx context.Context, req *deliveryv1.GetDeliveryRequest) (*deliveryv1.Delivery, error) {
	if req.GetOrderId() == "" {
		return nil, status.Error(codes.InvalidArgument, "order_id is required")
	}
	d, err := s.repo.Get(ctx, req.GetOrderId())
	if err == ErrNotFound {
		return nil, status.Errorf(codes.NotFound, "delivery for order %s not found", req.GetOrderId())
	}
	if err != nil {
		return nil, status.Errorf(codes.Internal, "get delivery: %v", err)
	}
	return d.toProto(), nil
}

type orderRef struct {
	OrderID string `json:"order_id"`
}

type dispatchedPayload struct {
	OrderID         string `json:"order_id"`
	FulfillmentType string `json:"fulfillment_type"`
	TrackingCode    string `json:"tracking_code"`
	Courier         string `json:"courier"`
	ETA             string `json:"eta"`
}

// HandleOrderEvent: на order.confirmed создаёт доставку под способ получения.
func (s *Service) HandleOrderEvent(ctx context.Context, env events.Envelope) error {
	if env.EventType != events.EventOrderConfirmed {
		return nil
	}
	var ref orderRef
	if err := env.UnmarshalPayload(&ref); err != nil {
		return err
	}

	exists, err := s.repo.Exists(ctx, ref.OrderID)
	if err != nil {
		return err
	}
	if exists {
		return nil // идемпотентность
	}

	// Способ получения берём из Order.
	ord, err := s.order.GetOrder(ctx, &orderv1.GetOrderRequest{Id: ref.OrderID})
	if err != nil {
		return err
	}
	fulfillment := fulfillmentString(ord.GetFulfillmentType())

	d := &Delivery{
		OrderID:         ref.OrderID,
		FulfillmentType: fulfillment,
		CorrelationID:   env.CorrelationID,
	}
	short := ref.OrderID
	if len(short) > 8 {
		short = short[:8]
	}

	// Тип события зависит от способа получения.
	var outEnv events.Envelope
	switch fulfillment {
	case order.FulfillmentShopDelivery:
		d.Status = StatusDispatched
		d.TrackingCode = "TEA-" + short
		d.ETA = "3-5 дней"
		outEnv, err = events.NewFollow(events.EventDeliveryDispatched, 1, ref.OrderID,
			dispatchedPayload{ref.OrderID, fulfillment, d.TrackingCode, "", d.ETA}, env)
	case order.FulfillmentFoodCourier:
		d.Status = StatusDispatched
		d.Courier = "Курьер #" + short
		d.ETA = "30-45 мин"
		outEnv, err = events.NewFollow(events.EventDeliveryDispatched, 1, ref.OrderID,
			dispatchedPayload{ref.OrderID, fulfillment, "", d.Courier, d.ETA}, env)
	default: // dine_in — доставка не нужна, сразу delivered
		d.Status = StatusDelivered
		outEnv, err = events.NewFollow(events.EventDeliveryDelivered, 1, ref.OrderID, orderRef{ref.OrderID}, env)
	}
	if err != nil {
		return err
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := insertTx(ctx, tx, d); err != nil {
		return fmt.Errorf("delivery: insert: %w", err)
	}
	if err := outbox.SaveTx(ctx, tx, outbox.SourceDelivery, events.TopicDelivery, outEnv); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return err
	}

	metrics.DeliveriesTotal.WithLabelValues(d.Status).Inc()
	s.log.Info("доставка создана", "order_id", ref.OrderID, "fulfillment", fulfillment, "status", d.Status)
	return nil
}

// AdvanceDispatched — один тик симулятора курьера: доводит dispatched-доставки
// до delivered и эмитит delivery.delivered (с тем же correlation_id саги).
func (s *Service) AdvanceDispatched(ctx context.Context) {
	items, err := s.repo.FetchDispatched(ctx, 50)
	if err != nil {
		s.log.Error("симулятор: выборка dispatched", "error", err)
		return
	}
	for _, it := range items {
		if err := s.deliver(ctx, it); err != nil {
			s.log.Error("симулятор: доставка", "order_id", it.OrderID, "error", err)
			return
		}
	}
}

func (s *Service) deliver(ctx context.Context, it Dispatched) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	ok, err := markDeliveredTx(ctx, tx, it.OrderID)
	if err != nil {
		return err
	}
	if !ok {
		return nil // уже доставлено — пропускаем
	}

	env, err := events.NewWithCorrelation(events.EventDeliveryDelivered, 1, it.OrderID, it.CorrelationID, orderRef{it.OrderID})
	if err != nil {
		return err
	}
	if err := outbox.SaveTx(ctx, tx, outbox.SourceDelivery, events.TopicDelivery, env); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return err
	}
	metrics.DeliveriesTotal.WithLabelValues(StatusDelivered).Inc()
	s.log.Info("заказ доставлен", "order_id", it.OrderID)
	return nil
}

// fulfillmentString переводит enum заказа в строку.
func fulfillmentString(ft orderv1.FulfillmentType) string {
	switch ft {
	case orderv1.FulfillmentType_FULFILLMENT_TYPE_SHOP_DELIVERY:
		return order.FulfillmentShopDelivery
	case orderv1.FulfillmentType_FULFILLMENT_TYPE_FOOD_COURIER:
		return order.FulfillmentFoodCourier
	case orderv1.FulfillmentType_FULFILLMENT_TYPE_DINE_IN:
		return order.FulfillmentDineIn
	default:
		return ""
	}
}
