package order

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	cartv1 "tea-platform/internal/genpb/cart/v1"
	orderv1 "tea-platform/internal/genpb/order/v1"
	"tea-platform/internal/metrics"
)

// Service реализует gRPC OrderService.
type Service struct {
	orderv1.UnimplementedOrderServiceServer

	repo *Repository
	cart cartv1.CartServiceClient
	log  *slog.Logger
}

// NewService собирает сервис заказов.
func NewService(repo *Repository, cart cartv1.CartServiceClient, log *slog.Logger) *Service {
	return &Service{repo: repo, cart: cart, log: log}
}

// CreateOrder оформляет заказ из текущей корзины (checkout): тянет позиции из
// Cart, формирует заказ, сохраняет его вместе с событием order.created (outbox)
// и очищает корзину.
func (s *Service) CreateOrder(ctx context.Context, req *orderv1.CreateOrderRequest) (*orderv1.Order, error) {
	start := time.Now()
	if req.GetCartId() == "" {
		return nil, status.Error(codes.InvalidArgument, "cart_id is required")
	}
	fulfillment := fulfillmentFromProto(req.GetFulfillmentType())
	if fulfillment == "" {
		return nil, status.Error(codes.InvalidArgument, "fulfillment_type is required")
	}

	cartResp, err := s.cart.GetCart(ctx, &cartv1.GetCartRequest{CartId: req.GetCartId()})
	if err != nil {
		return nil, status.Errorf(codes.Internal, "get cart: %v", err)
	}

	o := &Order{
		UserID:          req.GetUserId(),
		CartID:          req.GetCartId(),
		FulfillmentType: fulfillment,
		Status:          StatusCreated,
		Currency:        cartResp.GetCurrency(),
		Address:         req.GetAddress(),
		BookingID:       req.GetBookingId(),
	}

	var total int64
	for _, ci := range cartResp.GetItems() {
		if !ci.GetAvailable() {
			continue // недоступные позиции в заказ не попадают
		}
		o.Items = append(o.Items, Item{
			ProductID:      ci.GetProductId(),
			Name:           ci.GetName(),
			Quantity:       ci.GetQuantity(),
			UnitPriceCents: ci.GetUnitPriceCents(),
			SubtotalCents:  ci.GetSubtotalCents(),
		})
		total += ci.GetSubtotalCents()
	}
	if len(o.Items) == 0 {
		return nil, status.Error(codes.FailedPrecondition, "cart is empty")
	}
	o.TotalCents = total

	if err := s.repo.Create(ctx, o); err != nil {
		return nil, status.Errorf(codes.Internal, "create order: %v", err)
	}

	// Корзина больше не нужна — очищаем (best-effort, заказ уже создан).
	if _, err := s.cart.ClearCart(ctx, &cartv1.ClearCartRequest{CartId: req.GetCartId()}); err != nil {
		s.log.Warn("не удалось очистить корзину после оформления", "cart_id", req.GetCartId(), "error", err)
	}

	metrics.OrdersCreated.WithLabelValues(o.FulfillmentType).Inc()
	metrics.OrderStatus.WithLabelValues(StatusCreated).Inc()
	metrics.CheckoutDuration.Observe(time.Since(start).Seconds())

	s.log.Info("заказ создан",
		"order_id", o.ID, "total_cents", o.TotalCents,
		"fulfillment", o.FulfillmentType, "items", len(o.Items))
	return o.toProto(), nil
}

// ListOrders возвращает заказы пользователя для истории кабинета.
func (s *Service) ListOrders(ctx context.Context, req *orderv1.ListOrdersRequest) (*orderv1.ListOrdersResponse, error) {
	if req.GetUserId() == "" {
		return nil, status.Error(codes.InvalidArgument, "user_id is required")
	}
	orders, err := s.repo.ListByUser(ctx, req.GetUserId(), int(req.GetLimit()))
	if err != nil {
		return nil, status.Errorf(codes.Internal, "list orders: %v", err)
	}
	out := make([]*orderv1.Order, 0, len(orders))
	for _, o := range orders {
		out = append(out, o.toProto())
	}
	return &orderv1.ListOrdersResponse{Orders: out}, nil
}

// GetOrder возвращает заказ по идентификатору.
func (s *Service) GetOrder(ctx context.Context, req *orderv1.GetOrderRequest) (*orderv1.Order, error) {
	if req.GetId() == "" {
		return nil, status.Error(codes.InvalidArgument, "id is required")
	}
	o, err := s.repo.Get(ctx, req.GetId())
	if errors.Is(err, ErrNotFound) {
		return nil, status.Errorf(codes.NotFound, "order %s not found", req.GetId())
	}
	if err != nil {
		return nil, status.Errorf(codes.Internal, "get order: %v", err)
	}
	return o.toProto(), nil
}
