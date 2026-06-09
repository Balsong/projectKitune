// Command payment-svc — mock платёжного провайдера. Слушает inventory.events
// (stock.reserved), инициирует оплату и эмитит payment.succeeded/failed.
package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	"tea-platform/internal/config"
	"tea-platform/internal/db"
	orderv1 "tea-platform/internal/genpb/order/v1"
	"tea-platform/internal/kafka"
	"tea-platform/internal/metrics"
	"tea-platform/internal/outbox"
	"tea-platform/internal/payment"
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

	if err := kafka.EnsureTopics(ctx, cfg.KafkaBrokers, events.TopicInventory, events.TopicPayments); err != nil {
		log.Error("не удалось создать топики", "error", err)
		os.Exit(1)
	}

	outboxRepo := outbox.NewRepo(pool)
	relay := outbox.NewRelay(outboxRepo, producer, log, outbox.SourcePayment)
	go relay.Run(ctx)

	// gRPC-клиент к Order (за суммой заказа).
	orderConn, err := grpc.NewClient(cfg.OrderAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		log.Error("не удалось создать gRPC-клиент Order", "error", err)
		os.Exit(1)
	}
	defer func() { _ = orderConn.Close() }()
	orderClient := orderv1.NewOrderServiceClient(orderConn)

	svc := payment.NewService(pool, outboxRepo, orderClient, cfg.PaymentLimitCents, log)
	consumer := kafka.NewConsumer(cfg.KafkaBrokers, events.TopicInventory, "payment-svc", log)
	defer func() { _ = consumer.Close() }()

	log.Info("payment-svc запущен", "topic", events.TopicInventory, "limit_cents", cfg.PaymentLimitCents)
	if err := consumer.Run(ctx, svc.HandleStockReserved); err != nil {
		log.Error("консьюмер остановлен с ошибкой", "error", err)
		os.Exit(1)
	}
}
