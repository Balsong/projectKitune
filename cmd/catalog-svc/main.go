// Command catalog-svc — gRPC-сервис каталога: товары магазина (чай) и блюда
// ресторана. Применяет миграции при старте, читает из PostgreSQL с кэшем в Redis.
package main

import (
	"context"
	"errors"
	"log/slog"
	"net"
	"os"
	"os/signal"
	"syscall"

	"google.golang.org/grpc"
	"google.golang.org/grpc/health"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/reflection"

	"tea-platform/internal/cache"
	"tea-platform/internal/catalog"
	"tea-platform/internal/config"
	"tea-platform/internal/db"
	catalogv1 "tea-platform/internal/genpb/catalog/v1"
	"tea-platform/migrations"
)

func main() {
	log := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	cfg := config.Load()
	ctx := context.Background()

	// PostgreSQL + миграции.
	pool, err := db.NewPool(ctx, cfg.DSN())
	if err != nil {
		log.Error("не удалось подключиться к БД", "error", err)
		os.Exit(1)
	}
	defer pool.Close()

	if err := db.Migrate(ctx, pool, migrations.FS); err != nil {
		log.Error("не удалось применить миграции", "error", err)
		os.Exit(1)
	}

	// Redis для read-through кэша.
	rdb, err := cache.NewRedis(ctx, cfg.RedisAddr)
	if err != nil {
		log.Error("не удалось подключиться к Redis", "error", err)
		os.Exit(1)
	}
	defer func() { _ = rdb.Close() }()

	// gRPC-сервер.
	svc := catalog.NewService(catalog.NewRepository(pool), rdb, log)
	grpcServer := grpc.NewServer()
	catalogv1.RegisterCatalogServiceServer(grpcServer, svc)

	healthSrv := health.NewServer()
	healthSrv.SetServingStatus("catalog.v1.CatalogService", healthpb.HealthCheckResponse_SERVING)
	healthpb.RegisterHealthServer(grpcServer, healthSrv)
	reflection.Register(grpcServer) // удобно для grpcurl на этапе разработки

	lis, err := net.Listen("tcp", ":"+cfg.GRPCPort)
	if err != nil {
		log.Error("не удалось открыть порт", "port", cfg.GRPCPort, "error", err)
		os.Exit(1)
	}

	// Graceful shutdown по сигналу.
	go func() {
		sigCtx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
		defer stop()
		<-sigCtx.Done()
		log.Info("останавливаю catalog-svc")
		grpcServer.GracefulStop()
	}()

	log.Info("catalog-svc запущен", "grpc_port", cfg.GRPCPort)
	if err := grpcServer.Serve(lis); err != nil && !errors.Is(err, grpc.ErrServerStopped) {
		log.Error("ошибка gRPC-сервера", "error", err)
		os.Exit(1)
	}
}
