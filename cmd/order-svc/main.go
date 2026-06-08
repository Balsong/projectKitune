// Command order-svc — gRPC-сервис заказов. Создаёт заказ из корзины, пишет
// событие order.created в outbox в одной транзакции и публикует его в Kafka
// фоновым relay-воркером.
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

	"tea-platform/internal/config"
	"tea-platform/internal/db"
	cartv1 "tea-platform/internal/genpb/cart/v1"
	orderv1 "tea-platform/internal/genpb/order/v1"
	"tea-platform/internal/kafka"
	"tea-platform/internal/order"
	"tea-platform/internal/outbox"
	"tea-platform/migrations"
)

func main() {
	log := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	cfg := config.Load()

	// Корневой контекст: отменяется по сигналу и гасит relay-воркер.
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

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

	// Kafka-продюсер для relay.
	producer, err := kafka.NewProducer(cfg.KafkaBrokers)
	if err != nil {
		log.Error("не удалось инициализировать Kafka", "error", err)
		os.Exit(1)
	}
	defer func() { _ = producer.Close() }()

	// Outbox + relay-воркер (публикует order.created в Kafka).
	outboxRepo := outbox.NewRepo(pool)
	relay := outbox.NewRelay(outboxRepo, producer, log)
	go relay.Run(ctx)

	// gRPC-клиент к Cart.
	cartConn, err := grpc.NewClient(cfg.CartAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		log.Error("не удалось создать gRPC-клиент Cart", "error", err)
		os.Exit(1)
	}
	defer func() { _ = cartConn.Close() }()
	cartClient := cartv1.NewCartServiceClient(cartConn)

	// gRPC-сервер заказов.
	svc := order.NewService(order.NewRepository(pool), cartClient, log)
	grpcServer := grpc.NewServer()
	orderv1.RegisterOrderServiceServer(grpcServer, svc)

	healthSrv := health.NewServer()
	healthSrv.SetServingStatus("order.v1.OrderService", healthpb.HealthCheckResponse_SERVING)
	healthpb.RegisterHealthServer(grpcServer, healthSrv)
	reflection.Register(grpcServer)

	lis, err := net.Listen("tcp", ":"+cfg.GRPCPort)
	if err != nil {
		log.Error("не удалось открыть порт", "port", cfg.GRPCPort, "error", err)
		os.Exit(1)
	}

	go func() {
		<-ctx.Done()
		log.Info("останавливаю order-svc")
		grpcServer.GracefulStop()
	}()

	log.Info("order-svc запущен", "grpc_port", cfg.GRPCPort, "cart", cfg.CartAddr)
	if err := grpcServer.Serve(lis); err != nil && !errors.Is(err, grpc.ErrServerStopped) {
		log.Error("ошибка gRPC-сервера", "error", err)
		os.Exit(1)
	}
}
