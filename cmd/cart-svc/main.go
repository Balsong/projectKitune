// Command cart-svc — gRPC-сервис корзины: состояние в Redis, обогащение
// позиций данными из Catalog по gRPC.
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
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/health"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/reflection"

	"tea-platform/internal/cache"
	"tea-platform/internal/cart"
	"tea-platform/internal/config"
	cartv1 "tea-platform/internal/genpb/cart/v1"
	catalogv1 "tea-platform/internal/genpb/catalog/v1"
)

func main() {
	log := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	cfg := config.Load()
	ctx := context.Background()

	// Redis для хранения корзин.
	rdb, err := cache.NewRedis(ctx, cfg.RedisAddr)
	if err != nil {
		log.Error("не удалось подключиться к Redis", "error", err)
		os.Exit(1)
	}
	defer func() { _ = rdb.Close() }()

	// gRPC-клиент к Catalog для обогащения позиций.
	catalogConn, err := grpc.NewClient(
		cfg.CatalogAddr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		log.Error("не удалось создать gRPC-клиент Catalog", "error", err)
		os.Exit(1)
	}
	defer func() { _ = catalogConn.Close() }()
	catalogClient := catalogv1.NewCatalogServiceClient(catalogConn)

	// gRPC-сервер корзины.
	svc := cart.NewService(rdb, catalogClient, log)
	grpcServer := grpc.NewServer()
	cartv1.RegisterCartServiceServer(grpcServer, svc)

	healthSrv := health.NewServer()
	healthSrv.SetServingStatus("cart.v1.CartService", healthpb.HealthCheckResponse_SERVING)
	healthpb.RegisterHealthServer(grpcServer, healthSrv)
	reflection.Register(grpcServer)

	lis, err := net.Listen("tcp", ":"+cfg.GRPCPort)
	if err != nil {
		log.Error("не удалось открыть порт", "port", cfg.GRPCPort, "error", err)
		os.Exit(1)
	}

	go func() {
		sigCtx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
		defer stop()
		<-sigCtx.Done()
		log.Info("останавливаю cart-svc")
		grpcServer.GracefulStop()
	}()

	log.Info("cart-svc запущен", "grpc_port", cfg.GRPCPort, "catalog", cfg.CatalogAddr)
	if err := grpcServer.Serve(lis); err != nil && !errors.Is(err, grpc.ErrServerStopped) {
		log.Error("ошибка gRPC-сервера", "error", err)
		os.Exit(1)
	}
}
