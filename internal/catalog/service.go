package catalog

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/redis/go-redis/v9"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	catalogv1 "tea-platform/internal/genpb/catalog/v1"
)

// cacheTTL — время жизни кэша списков каталога. Каталог меняется редко,
// поэтому держим относительно долго; событийную инвалидацию добавим позже.
const cacheTTL = 10 * time.Minute

// Service реализует gRPC CatalogService поверх репозитория с кэшем в Redis.
type Service struct {
	catalogv1.UnimplementedCatalogServiceServer

	repo  *Repository
	cache *redis.Client
	log   *slog.Logger
}

// NewService собирает gRPC-сервис каталога.
func NewService(repo *Repository, cache *redis.Client, log *slog.Logger) *Service {
	return &Service{repo: repo, cache: cache, log: log}
}

// ListProducts возвращает позиции каталога по схеме read-through:
// сначала Redis, при промахе — PostgreSQL с последующим прогревом кэша.
func (s *Service) ListProducts(
	ctx context.Context,
	req *catalogv1.ListProductsRequest,
) (*catalogv1.ListProductsResponse, error) {
	kind := kindFromProto(req.GetKind())
	category := req.GetCategory()
	key := fmt.Sprintf("catalog:list:%s:%s", kind, category)

	// 1. Пытаемся прочитать из кэша.
	if products, ok := s.cacheGet(ctx, key); ok {
		return toListResponse(products), nil
	}

	// 2. Промах кэша — идём в БД.
	products, err := s.repo.List(ctx, kind, category)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "list products: %v", err)
	}

	// 3. Прогреваем кэш (best-effort).
	s.cacheSet(ctx, key, products)

	return toListResponse(products), nil
}

// GetProduct возвращает одну позицию по идентификатору.
func (s *Service) GetProduct(
	ctx context.Context,
	req *catalogv1.GetProductRequest,
) (*catalogv1.GetProductResponse, error) {
	if req.GetId() == "" {
		return nil, status.Error(codes.InvalidArgument, "id is required")
	}

	product, err := s.repo.Get(ctx, req.GetId())
	if errors.Is(err, ErrNotFound) {
		return nil, status.Errorf(codes.NotFound, "product %s not found", req.GetId())
	}
	if err != nil {
		return nil, status.Errorf(codes.Internal, "get product: %v", err)
	}

	return &catalogv1.GetProductResponse{Product: product.toProto()}, nil
}

// UpdateProduct меняет цену и доступность позиции (админка) и сбрасывает кэш.
func (s *Service) UpdateProduct(
	ctx context.Context,
	req *catalogv1.UpdateProductRequest,
) (*catalogv1.Product, error) {
	if req.GetId() == "" {
		return nil, status.Error(codes.InvalidArgument, "id is required")
	}
	if req.GetPriceCents() < 0 {
		return nil, status.Error(codes.InvalidArgument, "price_cents must be >= 0")
	}

	product, err := s.repo.Update(ctx, req.GetId(), req.GetPriceCents(), req.GetAvailable())
	if errors.Is(err, ErrNotFound) {
		return nil, status.Errorf(codes.NotFound, "product %s not found", req.GetId())
	}
	if err != nil {
		return nil, status.Errorf(codes.Internal, "update product: %v", err)
	}

	s.invalidateLists(ctx)
	return product.toProto(), nil
}

// invalidateLists сбрасывает кэш списков каталога (best-effort).
func (s *Service) invalidateLists(ctx context.Context) {
	keys, err := s.cache.Keys(ctx, "catalog:list:*").Result()
	if err != nil {
		s.log.Warn("cache keys failed", "error", err)
		return
	}
	if len(keys) > 0 {
		if err := s.cache.Del(ctx, keys...).Err(); err != nil {
			s.log.Warn("cache del failed", "error", err)
		}
	}
}

// cacheGet читает список из Redis. Возвращает ok=false при промахе или ошибке
// (кэш — best-effort, ошибки не фатальны).
func (s *Service) cacheGet(ctx context.Context, key string) ([]Product, bool) {
	raw, err := s.cache.Get(ctx, key).Bytes()
	if errors.Is(err, redis.Nil) {
		return nil, false
	}
	if err != nil {
		s.log.Warn("cache get failed", "key", key, "error", err)
		return nil, false
	}

	var products []Product
	if err := json.Unmarshal(raw, &products); err != nil {
		s.log.Warn("cache unmarshal failed", "key", key, "error", err)
		return nil, false
	}
	return products, true
}

// cacheSet кладёт список в Redis с TTL. Ошибки только логируются.
func (s *Service) cacheSet(ctx context.Context, key string, products []Product) {
	raw, err := json.Marshal(products)
	if err != nil {
		s.log.Warn("cache marshal failed", "key", key, "error", err)
		return
	}
	if err := s.cache.Set(ctx, key, raw, cacheTTL).Err(); err != nil {
		s.log.Warn("cache set failed", "key", key, "error", err)
	}
}

func toListResponse(products []Product) *catalogv1.ListProductsResponse {
	out := make([]*catalogv1.Product, 0, len(products))
	for _, p := range products {
		out = append(out, p.toProto())
	}
	return &catalogv1.ListProductsResponse{Products: out}
}
