package shop

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	shopv1 "tea-platform/internal/genpb/shop/v1"
	"tea-platform/internal/outbox"
	"tea-platform/pkg/events"
)

// Service реализует gRPC ShopService.
type Service struct {
	shopv1.UnimplementedShopServiceServer

	pool              *pgxpool.Pool
	repo              *Repository
	outbox            *outbox.Repo
	log               *slog.Logger
	shipFlatCents     int64
	shipFreeThanCents int64
}

// NewService собирает сервис магазина. shipFlat — единый тариф доставки по
// стране, shipFreeThan — порог бесплатной доставки (копейки).
func NewService(pool *pgxpool.Pool, repo *Repository, outboxRepo *outbox.Repo, log *slog.Logger, shipFlat, shipFreeThan int64) *Service {
	return &Service{
		pool: pool, repo: repo, outbox: outboxRepo, log: log,
		shipFlatCents: shipFlat, shipFreeThanCents: shipFreeThan,
	}
}

// ---- Каталог ----

func (s *Service) ListProducts(ctx context.Context, req *shopv1.ListProductsRequest) (*shopv1.ListProductsResponse, error) {
	products, err := s.repo.ListProducts(ctx, req.GetCategory())
	if err != nil {
		return nil, status.Errorf(codes.Internal, "list products: %v", err)
	}
	out := make([]*shopv1.Product, 0, len(products))
	for _, p := range products {
		out = append(out, p.toProto())
	}
	return &shopv1.ListProductsResponse{Products: out}, nil
}

func (s *Service) GetProduct(ctx context.Context, req *shopv1.GetProductRequest) (*shopv1.Product, error) {
	p, err := s.repo.GetProduct(ctx, req.GetId())
	if errors.Is(err, ErrNotFound) {
		return nil, status.Errorf(codes.NotFound, "product %s not found", req.GetId())
	}
	if err != nil {
		return nil, status.Errorf(codes.Internal, "get product: %v", err)
	}
	return p.toProto(), nil
}

func (s *Service) UpdateProduct(ctx context.Context, req *shopv1.UpdateProductRequest) (*shopv1.Product, error) {
	if req.GetId() == "" || req.GetPriceCents() < 0 || req.GetStockQty() < 0 {
		return nil, status.Error(codes.InvalidArgument, "id required, price/stock must be >= 0")
	}
	p, err := s.repo.UpdateProduct(ctx, req.GetId(), req.GetPriceCents(), req.GetStockQty(), req.GetAvailable())
	if errors.Is(err, ErrNotFound) {
		return nil, status.Errorf(codes.NotFound, "product %s not found", req.GetId())
	}
	if err != nil {
		return nil, status.Errorf(codes.Internal, "update product: %v", err)
	}
	return p.toProto(), nil
}

// ---- Корзина ----

func (s *Service) cart(ctx context.Context, cartID string) (*shopv1.Cart, error) {
	lines, err := s.repo.CartLines(ctx, cartID)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "cart: %v", err)
	}
	items := make([]*shopv1.CartItem, 0, len(lines))
	var total int64
	for _, l := range lines {
		items = append(items, l.toProto())
		total += l.SubtotalCents
	}
	return &shopv1.Cart{CartId: cartID, Items: items, TotalCents: total}, nil
}

func (s *Service) GetCart(ctx context.Context, req *shopv1.CartRequest) (*shopv1.Cart, error) {
	if req.GetCartId() == "" {
		return nil, status.Error(codes.InvalidArgument, "cart_id is required")
	}
	return s.cart(ctx, req.GetCartId())
}

func (s *Service) AddToCart(ctx context.Context, req *shopv1.AddToCartRequest) (*shopv1.Cart, error) {
	if req.GetCartId() == "" || req.GetProductId() == "" {
		return nil, status.Error(codes.InvalidArgument, "cart_id and product_id are required")
	}
	if err := s.repo.AddToCart(ctx, req.GetCartId(), req.GetProductId(), req.GetQuantity()); err != nil {
		if errors.Is(err, ErrNotFound) {
			return nil, status.Error(codes.NotFound, "товар не найден")
		}
		return nil, status.Errorf(codes.Internal, "add to cart: %v", err)
	}
	return s.cart(ctx, req.GetCartId())
}

func (s *Service) RemoveFromCart(ctx context.Context, req *shopv1.RemoveFromCartRequest) (*shopv1.Cart, error) {
	if req.GetCartId() == "" || req.GetProductId() == "" {
		return nil, status.Error(codes.InvalidArgument, "cart_id and product_id are required")
	}
	if err := s.repo.RemoveFromCart(ctx, req.GetCartId(), req.GetProductId(), req.GetQuantity()); err != nil {
		return nil, status.Errorf(codes.Internal, "remove from cart: %v", err)
	}
	return s.cart(ctx, req.GetCartId())
}

func (s *Service) ClearCart(ctx context.Context, req *shopv1.CartRequest) (*shopv1.Cart, error) {
	if req.GetCartId() == "" {
		return nil, status.Error(codes.InvalidArgument, "cart_id is required")
	}
	if err := s.repo.ClearCart(ctx, req.GetCartId()); err != nil {
		return nil, status.Errorf(codes.Internal, "clear cart: %v", err)
	}
	return s.cart(ctx, req.GetCartId())
}

// deliveryCents считает стоимость доставки: бесплатно от порога, иначе единый тариф.
func (s *Service) deliveryCents(subtotal int64) int64 {
	if subtotal >= s.shipFreeThanCents {
		return 0
	}
	return s.shipFlatCents
}

type shopOrderEventPayload struct {
	OrderID      string `json:"order_id"`
	UserID       string `json:"user_id"`
	Customer     string `json:"customer"`
	Email        string `json:"email"`
	TotalCents   int64  `json:"total_cents"`
	TrackingCode string `json:"tracking_code,omitempty"`
}

// Checkout оформляет заказ магазина: резервирует склад, считает доставку,
// «оплачивает» (mock — мгновенно), списывает остаток и чистит корзину — всё в
// одной транзакции БД tea_shop. Событие shop.order.paid пишется в outbox.
func (s *Service) Checkout(ctx context.Context, req *shopv1.CheckoutRequest) (*shopv1.Order, error) {
	customer := strings.TrimSpace(req.GetCustomer())
	if customer == "" || strings.TrimSpace(req.GetPhone()) == "" ||
		strings.TrimSpace(req.GetCity()) == "" || strings.TrimSpace(req.GetAddress()) == "" {
		return nil, status.Error(codes.InvalidArgument, "нужны имя, телефон, город и адрес доставки")
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "begin: %v", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	// Блокируем строки товаров корзины и проверяем остаток.
	const q = `
		SELECT p.id, p.name, p.price_cents, p.stock_qty, p.available, ci.quantity
		FROM cart_items ci JOIN products p ON p.id = ci.product_id
		WHERE ci.cart_id = $1 FOR UPDATE OF p`
	rows, err := tx.Query(ctx, q, req.GetCartId())
	if err != nil {
		return nil, status.Errorf(codes.Internal, "lock cart: %v", err)
	}
	type line struct {
		id, name   string
		price      int64
		stock, qty int32
	}
	var lines []line
	for rows.Next() {
		var l line
		var available bool
		if err := rows.Scan(&l.id, &l.name, &l.price, &l.stock, &available, &l.qty); err != nil {
			rows.Close()
			return nil, status.Errorf(codes.Internal, "scan cart: %v", err)
		}
		if !available || l.stock < l.qty {
			rows.Close()
			return nil, status.Errorf(codes.FailedPrecondition, "нет в наличии: %s", l.name)
		}
		lines = append(lines, l)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, status.Errorf(codes.Internal, "cart rows: %v", err)
	}
	if len(lines) == 0 {
		return nil, status.Error(codes.FailedPrecondition, "корзина пуста")
	}

	var subtotal int64
	for _, l := range lines {
		subtotal += l.price * int64(l.qty)
	}
	delivery := s.deliveryCents(subtotal)
	total := subtotal + delivery

	// Создаём заказ сразу оплаченным (mock-оплата мгновенна).
	const insOrder = `
		INSERT INTO orders (user_id, status, customer, phone, email, country, region, city,
			address, postal_code, comment, items_subtotal_cents, delivery_cents, total_cents)
		VALUES ($1,'paid',$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13)
		RETURNING id, created_at`
	country := strings.TrimSpace(req.GetCountry())
	if country == "" {
		country = "Россия"
	}
	var orderID string
	var createdAt time.Time
	if err := tx.QueryRow(ctx, insOrder, req.GetUserId(), customer, req.GetPhone(), req.GetEmail(),
		country, req.GetRegion(), req.GetCity(), req.GetAddress(), req.GetPostalCode(), req.GetComment(),
		subtotal, delivery, total).Scan(&orderID, &createdAt); err != nil {
		return nil, status.Errorf(codes.Internal, "insert order: %v", err)
	}

	// Позиции + списание остатка.
	for _, l := range lines {
		if _, err := tx.Exec(ctx,
			`INSERT INTO order_items (order_id, product_id, name, quantity, unit_price_cents, subtotal_cents)
			 VALUES ($1,$2,$3,$4,$5,$6)`,
			orderID, l.id, l.name, l.qty, l.price, l.price*int64(l.qty)); err != nil {
			return nil, status.Errorf(codes.Internal, "insert item: %v", err)
		}
		if _, err := tx.Exec(ctx,
			`UPDATE products SET stock_qty = stock_qty - $2 WHERE id = $1`, l.id, l.qty); err != nil {
			return nil, status.Errorf(codes.Internal, "decrement stock: %v", err)
		}
	}

	if _, err := tx.Exec(ctx, `DELETE FROM cart_items WHERE cart_id = $1`, req.GetCartId()); err != nil {
		return nil, status.Errorf(codes.Internal, "clear cart: %v", err)
	}

	env, err := events.New(events.EventShopOrderPaid, 1, orderID, shopOrderEventPayload{
		OrderID: orderID, UserID: req.GetUserId(), Customer: customer, Email: req.GetEmail(), TotalCents: total,
	})
	if err != nil {
		return nil, status.Errorf(codes.Internal, "build event: %v", err)
	}
	if err := outbox.SaveTx(ctx, tx, outbox.SourceShop, events.TopicShop, env); err != nil {
		return nil, status.Errorf(codes.Internal, "outbox: %v", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, status.Errorf(codes.Internal, "commit: %v", err)
	}

	s.log.Info("заказ магазина оплачен", "order_id", orderID, "user_id", req.GetUserId(),
		"total_cents", total, "items", len(lines))
	o, err := s.repo.GetOrder(ctx, orderID)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "load order: %v", err)
	}
	return o.toProto(), nil
}

func (s *Service) GetOrder(ctx context.Context, req *shopv1.GetOrderRequest) (*shopv1.Order, error) {
	o, err := s.repo.GetOrder(ctx, req.GetId())
	if errors.Is(err, ErrNotFound) {
		return nil, status.Errorf(codes.NotFound, "order %s not found", req.GetId())
	}
	if err != nil {
		return nil, status.Errorf(codes.Internal, "get order: %v", err)
	}
	return o.toProto(), nil
}

func (s *Service) ListOrders(ctx context.Context, req *shopv1.ListOrdersRequest) (*shopv1.ListOrdersResponse, error) {
	if req.GetUserId() == "" {
		return nil, status.Error(codes.InvalidArgument, "user_id is required")
	}
	orders, err := s.repo.ListByUser(ctx, req.GetUserId(), 0)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "list orders: %v", err)
	}
	return ordersResponse(orders), nil
}

func (s *Service) ListAllOrders(ctx context.Context, req *shopv1.ListAllOrdersRequest) (*shopv1.ListOrdersResponse, error) {
	orders, err := s.repo.ListAll(ctx, int(req.GetLimit()))
	if err != nil {
		return nil, status.Errorf(codes.Internal, "list all orders: %v", err)
	}
	return ordersResponse(orders), nil
}

var validShopStatus = map[string]bool{"created": true, "paid": true, "shipped": true, "delivered": true, "cancelled": true}

func (s *Service) UpdateOrderStatus(ctx context.Context, req *shopv1.UpdateOrderStatusRequest) (*shopv1.Order, error) {
	if req.GetId() == "" || !validShopStatus[req.GetStatus()] {
		return nil, status.Error(codes.InvalidArgument, "valid id and status are required")
	}
	tracking := strings.TrimSpace(req.GetTrackingCode())
	// При отправке без трека генерируем номер автоматически.
	if req.GetStatus() == "shipped" && tracking == "" {
		tracking = "KT" + strings.ToUpper(events.NewID()[:10])
	}
	o, err := s.repo.UpdateOrderStatus(ctx, req.GetId(), req.GetStatus(), tracking)
	if errors.Is(err, ErrNotFound) {
		return nil, status.Errorf(codes.NotFound, "order %s not found", req.GetId())
	}
	if err != nil {
		return nil, status.Errorf(codes.Internal, "update order: %v", err)
	}

	if req.GetStatus() == "shipped" {
		env, err := events.New(events.EventShopOrderShipped, 1, o.ID, shopOrderEventPayload{
			OrderID: o.ID, UserID: o.UserID, Customer: o.Customer, Email: o.Email,
			TotalCents: o.TotalCents, TrackingCode: o.TrackingCode,
		})
		if err == nil {
			if err := s.outbox.Save(ctx, outbox.SourceShop, events.TopicShop, env); err != nil {
				s.log.Warn("не удалось записать событие отправки", "order_id", o.ID, "error", err)
			}
		}
	}
	s.log.Info("статус заказа магазина изменён", "order_id", o.ID, "status", o.Status)
	return o.toProto(), nil
}

func ordersResponse(orders []*Order) *shopv1.ListOrdersResponse {
	out := make([]*shopv1.Order, 0, len(orders))
	for _, o := range orders {
		out = append(out, o.toProto())
	}
	return &shopv1.ListOrdersResponse{Orders: out}
}
