// Command delivery-svc — доставка. По order.confirmed создаёт доставку (трек/
// курьер по способу получения), фоновый симулятор доводит её до delivered.
// gRPC отдаёт статус доставки клиентам.
package main

import (
	"context"
	"errors"
	"log/slog"
	"net"
	"os"
	"os/signal"
	"syscall"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/health"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/reflection"

	"tea-platform/internal/config"
	"tea-platform/internal/db"
	"tea-platform/internal/delivery"
	deliveryv1 "tea-platform/internal/genpb/delivery/v1"
	orderv1 "tea-platform/internal/genpb/order/v1"
	"tea-platform/internal/kafka"
	"tea-platform/internal/metrics"
	"tea-platform/internal/outbox"
	"tea-platform/migrations"
	"tea-platform/pkg/events"
)

func main() {
	log := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	cfg := config.Load()

	go metrics.Serve(":"+cfg.MetricsPort, log)

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

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

	producer, err := kafka.NewProducer(cfg.KafkaBrokers)
	if err != nil {
		log.Error("не удалось инициализировать Kafka", "error", err)
		os.Exit(1)
	}
	defer func() { _ = producer.Close() }()

	if err := kafka.EnsureTopics(ctx, cfg.KafkaBrokers, events.TopicOrders, events.TopicDelivery); err != nil {
		log.Error("не удалось создать топики", "error", err)
		os.Exit(1)
	}

	outboxRepo := outbox.NewRepo(pool)
	relay := outbox.NewRelay(outboxRepo, producer, log, outbox.SourceDelivery)
	go relay.Run(ctx)

	// gRPC-клиент к Order (за способом получения).
	orderConn, err := grpc.NewClient(cfg.OrderAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		log.Error("не удалось создать gRPC-клиент Order", "error", err)
		os.Exit(1)
	}
	defer func() { _ = orderConn.Close() }()
	orderClient := orderv1.NewOrderServiceClient(orderConn)

	svc := delivery.NewService(pool, outboxRepo, orderClient, log)

	// Консьюмер order.confirmed.
	consumer := kafka.NewConsumer(cfg.KafkaBrokers, events.TopicOrders, "delivery-svc", log)
	defer func() { _ = consumer.Close() }()
	go func() {
		if err := consumer.Run(ctx, svc.HandleOrderEvent); err != nil {
			log.Error("консьюмер остановлен", "error", err)
		}
	}()

	// Симулятор курьера: доводит dispatched-доставки до delivered.
	go func() {
		ticker := time.NewTicker(3 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				svc.AdvanceDispatched(ctx)
			}
		}
	}()

	// gRPC-сервер.
	grpcServer := grpc.NewServer()
	deliveryv1.RegisterDeliveryServiceServer(grpcServer, svc)
	healthSrv := health.NewServer()
	healthSrv.SetServingStatus("delivery.v1.DeliveryService", healthpb.HealthCheckResponse_SERVING)
	healthpb.RegisterHealthServer(grpcServer, healthSrv)
	reflection.Register(grpcServer)

	lis, err := net.Listen("tcp", ":"+cfg.GRPCPort)
	if err != nil {
		log.Error("не удалось открыть порт", "port", cfg.GRPCPort, "error", err)
		os.Exit(1)
	}

	go func() {
		<-ctx.Done()
		log.Info("останавливаю delivery-svc")
		grpcServer.GracefulStop()
	}()

	log.Info("delivery-svc запущен", "grpc_port", cfg.GRPCPort)
	if err := grpcServer.Serve(lis); err != nil && !errors.Is(err, grpc.ErrServerStopped) {
		log.Error("ошибка gRPC-сервера", "error", err)
		os.Exit(1)
	}
}
