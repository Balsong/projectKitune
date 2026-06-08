// Package cart содержит gRPC-сервис корзины: состояние в Redis (Hash + TTL),
// обогащение позиций данными из Catalog (название, цена, итог).
package cart

import (
	"context"
	"log/slog"
	"sort"
	"strconv"
	"time"

	"github.com/redis/go-redis/v9"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	cartv1 "tea-platform/internal/genpb/cart/v1"
	catalogv1 "tea-platform/internal/genpb/catalog/v1"
)

// cartTTL — срок жизни корзины. Обновляется при каждой записи.
const cartTTL = 7 * 24 * time.Hour

// defaultCurrency используется, когда корзина пуста (нет товара, из которого
// взять валюту).
const defaultCurrency = "RUB"

// Service реализует gRPC CartService.
type Service struct {
	cartv1.UnimplementedCartServiceServer

	rdb     *redis.Client
	catalog catalogv1.CatalogServiceClient
	log     *slog.Logger
}

// NewService собирает сервис корзины.
func NewService(rdb *redis.Client, catalog catalogv1.CatalogServiceClient, log *slog.Logger) *Service {
	return &Service{rdb: rdb, catalog: catalog, log: log}
}

func cartKey(cartID string) string { return "cart:" + cartID }

// AddItem добавляет позицию в корзину, предварительно проверив, что товар
// существует в каталоге.
func (s *Service) AddItem(ctx context.Context, req *cartv1.AddItemRequest) (*cartv1.Cart, error) {
	if req.GetCartId() == "" || req.GetProductId() == "" {
		return nil, status.Error(codes.InvalidArgument, "cart_id and product_id are required")
	}
	if req.GetQuantity() <= 0 {
		return nil, status.Error(codes.InvalidArgument, "quantity must be positive")
	}

	// Валидируем существование товара через Catalog.
	if _, err := s.catalog.GetProduct(ctx, &catalogv1.GetProductRequest{Id: req.GetProductId()}); err != nil {
		if status.Code(err) == codes.NotFound {
			return nil, status.Errorf(codes.InvalidArgument, "product %s not found", req.GetProductId())
		}
		return nil, status.Errorf(codes.Internal, "validate product: %v", err)
	}

	key := cartKey(req.GetCartId())
	if err := s.rdb.HIncrBy(ctx, key, req.GetProductId(), int64(req.GetQuantity())).Err(); err != nil {
		return nil, status.Errorf(codes.Internal, "add item: %v", err)
	}
	s.touchTTL(ctx, key)

	return s.buildCart(ctx, req.GetCartId())
}

// RemoveItem уменьшает количество; при quantity <= 0 или достижении нуля
// позиция удаляется целиком.
func (s *Service) RemoveItem(ctx context.Context, req *cartv1.RemoveItemRequest) (*cartv1.Cart, error) {
	if req.GetCartId() == "" || req.GetProductId() == "" {
		return nil, status.Error(codes.InvalidArgument, "cart_id and product_id are required")
	}

	key := cartKey(req.GetCartId())

	if req.GetQuantity() <= 0 {
		if err := s.rdb.HDel(ctx, key, req.GetProductId()).Err(); err != nil {
			return nil, status.Errorf(codes.Internal, "remove item: %v", err)
		}
		return s.buildCart(ctx, req.GetCartId())
	}

	newQty, err := s.rdb.HIncrBy(ctx, key, req.GetProductId(), -int64(req.GetQuantity())).Result()
	if err != nil {
		return nil, status.Errorf(codes.Internal, "remove item: %v", err)
	}
	if newQty <= 0 {
		if err := s.rdb.HDel(ctx, key, req.GetProductId()).Err(); err != nil {
			return nil, status.Errorf(codes.Internal, "remove item: %v", err)
		}
	}
	s.touchTTL(ctx, key)

	return s.buildCart(ctx, req.GetCartId())
}

// GetCart возвращает содержимое корзины.
func (s *Service) GetCart(ctx context.Context, req *cartv1.GetCartRequest) (*cartv1.Cart, error) {
	if req.GetCartId() == "" {
		return nil, status.Error(codes.InvalidArgument, "cart_id is required")
	}
	return s.buildCart(ctx, req.GetCartId())
}

// ClearCart полностью очищает корзину.
func (s *Service) ClearCart(ctx context.Context, req *cartv1.ClearCartRequest) (*cartv1.Cart, error) {
	if req.GetCartId() == "" {
		return nil, status.Error(codes.InvalidArgument, "cart_id is required")
	}
	if err := s.rdb.Del(ctx, cartKey(req.GetCartId())).Err(); err != nil {
		return nil, status.Errorf(codes.Internal, "clear cart: %v", err)
	}
	return &cartv1.Cart{CartId: req.GetCartId(), Currency: defaultCurrency}, nil
}

// MergeCart переносит позиции гостевой корзины в пользовательскую и удаляет
// гостевую. Применяется при логине.
func (s *Service) MergeCart(ctx context.Context, req *cartv1.MergeCartRequest) (*cartv1.Cart, error) {
	if req.GetSourceCartId() == "" || req.GetTargetCartId() == "" {
		return nil, status.Error(codes.InvalidArgument, "source_cart_id and target_cart_id are required")
	}

	srcKey := cartKey(req.GetSourceCartId())
	dstKey := cartKey(req.GetTargetCartId())

	items, err := s.rdb.HGetAll(ctx, srcKey).Result()
	if err != nil {
		return nil, status.Errorf(codes.Internal, "merge cart: %v", err)
	}

	for productID, qtyStr := range items {
		qty, convErr := strconv.ParseInt(qtyStr, 10, 64)
		if convErr != nil {
			continue // битое значение пропускаем
		}
		if err := s.rdb.HIncrBy(ctx, dstKey, productID, qty).Err(); err != nil {
			return nil, status.Errorf(codes.Internal, "merge cart: %v", err)
		}
	}

	if len(items) > 0 {
		s.touchTTL(ctx, dstKey)
	}
	if err := s.rdb.Del(ctx, srcKey).Err(); err != nil {
		s.log.Warn("не удалось удалить гостевую корзину после merge", "key", srcKey, "error", err)
	}

	return s.buildCart(ctx, req.GetTargetCartId())
}

// touchTTL продлевает срок жизни корзины. Ошибки только логируются.
func (s *Service) touchTTL(ctx context.Context, key string) {
	if err := s.rdb.Expire(ctx, key, cartTTL).Err(); err != nil {
		s.log.Warn("не удалось обновить TTL корзины", "key", key, "error", err)
	}
}

// buildCart читает позиции из Redis и обогащает их данными из Catalog.
func (s *Service) buildCart(ctx context.Context, cartID string) (*cartv1.Cart, error) {
	raw, err := s.rdb.HGetAll(ctx, cartKey(cartID)).Result()
	if err != nil {
		return nil, status.Errorf(codes.Internal, "read cart: %v", err)
	}

	cart := &cartv1.Cart{CartId: cartID, Currency: defaultCurrency}
	var total int64

	for productID, qtyStr := range raw {
		qty, convErr := strconv.ParseInt(qtyStr, 10, 32)
		if convErr != nil || qty <= 0 {
			continue
		}

		item := &cartv1.CartItem{ProductId: productID, Quantity: int32(qty)}

		resp, getErr := s.catalog.GetProduct(ctx, &catalogv1.GetProductRequest{Id: productID})
		switch {
		case getErr == nil:
			p := resp.GetProduct()
			item.Name = p.GetName()
			item.UnitPriceCents = p.GetPriceCents()
			item.SubtotalCents = p.GetPriceCents() * qty
			item.Available = p.GetAvailable()
			if p.GetCurrency() != "" {
				cart.Currency = p.GetCurrency()
			}
			if item.Available {
				total += item.SubtotalCents
			}
		case status.Code(getErr) == codes.NotFound:
			// Товар исчез из каталога — показываем недоступным, не считаем в сумму.
			item.Available = false
		default:
			return nil, status.Errorf(codes.Internal, "enrich item %s: %v", productID, getErr)
		}

		cart.Items = append(cart.Items, item)
	}

	// Стабильный порядок вывода — по названию, затем по product_id.
	sort.Slice(cart.Items, func(i, j int) bool {
		if cart.Items[i].GetName() != cart.Items[j].GetName() {
			return cart.Items[i].GetName() < cart.Items[j].GetName()
		}
		return cart.Items[i].GetProductId() < cart.Items[j].GetProductId()
	})

	cart.TotalCents = total
	return cart, nil
}
