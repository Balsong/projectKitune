// Command inventory-svc — событийный сервис инвентаря. Слушает orders.events,
// резервирует остатки под order.created и эмитит stock.reserved /
// reservation.failed через transactional outbox (relay-воркер).
package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"tea-platform/internal/config"
	"tea-platform/internal/db"
	"tea-platform/internal/inventory"
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

	// Kafka-продюсер + outbox relay (публикует inventory.events).
	producer, err := kafka.NewProducer(cfg.KafkaBrokers)
	if err != nil {
		log.Error("не удалось инициализировать Kafka", "error", err)
		os.Exit(1)
	}
	defer func() { _ = producer.Close() }()

	// Гарантируем существование топиков до запуска консьюмера/relay.
	if err := kafka.EnsureTopics(ctx, cfg.KafkaBrokers, events.TopicOrders, events.TopicInventory); err != nil {
		log.Error("не удалось создать топики", "error", err)
		os.Exit(1)
	}

	outboxRepo := outbox.NewRepo(pool)
	relay := outbox.NewRelay(outboxRepo, producer, log, outbox.SourceInventory)
	go relay.Run(ctx)

	// Консьюмер orders.events → резервирование.
	svc := inventory.NewService(pool, outboxRepo, log)
	consumer := kafka.NewConsumer(cfg.KafkaBrokers, events.TopicOrders, "inventory-svc", log)
	defer func() { _ = consumer.Close() }()

	log.Info("inventory-svc запущен", "topic", events.TopicOrders)
	if err := consumer.Run(ctx, svc.HandleOrderEvent); err != nil {
		log.Error("консьюмер остановлен с ошибкой", "error", err)
		os.Exit(1)
	}
}
