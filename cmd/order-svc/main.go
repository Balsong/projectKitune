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
	"tea-platform/pkg/events"
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

	// Топики саги: создаём до запуска консьюмеров/relay.
	if err := kafka.EnsureTopics(ctx, cfg.KafkaBrokers,
		events.TopicOrders, events.TopicInventory, events.TopicPayments, events.TopicDelivery); err != nil {
		log.Error("не удалось создать топики", "error", err)
		os.Exit(1)
	}

	// Outbox + relay-воркер (публикует события заказа в Kafka).
	outboxRepo := outbox.NewRepo(pool)
	relay := outbox.NewRelay(outboxRepo, producer, log, outbox.SourceOrder)
	go relay.Run(ctx)

	// gRPC-клиент к Cart.
	cartConn, err := grpc.NewClient(cfg.CartAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		log.Error("не удалось создать gRPC-клиент Cart", "error", err)
		os.Exit(1)
	}
	defer func() { _ = cartConn.Close() }()
	cartClient := cartv1.NewCartServiceClient(cartConn)

	repo := order.NewRepository(pool)

	// Дирижёр саги: слушает inventory.events и payments.events, продвигает
	// state machine заказа и эмитит order.confirmed / order.cancelled.
	orch := order.NewOrchestrator(repo, log)
	invConsumer := kafka.NewConsumer(cfg.KafkaBrokers, events.TopicInventory, "order-svc-inventory", log)
	payConsumer := kafka.NewConsumer(cfg.KafkaBrokers, events.TopicPayments, "order-svc-payments", log)
	delConsumer := kafka.NewConsumer(cfg.KafkaBrokers, events.TopicDelivery, "order-svc-delivery", log)
	defer func() { _ = invConsumer.Close() }()
	defer func() { _ = payConsumer.Close() }()
	defer func() { _ = delConsumer.Close() }()
	go func() {
		if err := invConsumer.Run(ctx, orch.HandleInventoryEvent); err != nil {
			log.Error("консьюмер inventory остановлен", "error", err)
		}
	}()
	go func() {
		if err := payConsumer.Run(ctx, orch.HandlePaymentEvent); err != nil {
			log.Error("консьюмер payments остановлен", "error", err)
		}
	}()
	go func() {
		if err := delConsumer.Run(ctx, orch.HandleDeliveryEvent); err != nil {
			log.Error("консьюмер delivery остановлен", "error", err)
		}
	}()

	// gRPC-сервер заказов.
	svc := order.NewService(repo, cartClient, log)
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
