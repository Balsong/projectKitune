// Command booking-svc — gRPC-сервис броней. Хранит брони в Postgres и эмитит
// booking.requested через outbox (relay → Kafka → notification).
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

	"tea-platform/internal/booking"
	"tea-platform/internal/config"
	"tea-platform/internal/db"
	bookingv1 "tea-platform/internal/genpb/booking/v1"
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
	if err := kafka.EnsureTopics(ctx, cfg.KafkaBrokers, events.TopicBookings); err != nil {
		log.Error("не удалось создать топики", "error", err)
		os.Exit(1)
	}

	outboxRepo := outbox.NewRepo(pool)
	relay := outbox.NewRelay(outboxRepo, producer, log, outbox.SourceBooking)
	go relay.Run(ctx)

	svc := booking.NewService(pool, outboxRepo, log)
	grpcServer := grpc.NewServer()
	bookingv1.RegisterBookingServiceServer(grpcServer, svc)

	healthSrv := health.NewServer()
	healthSrv.SetServingStatus("booking.v1.BookingService", healthpb.HealthCheckResponse_SERVING)
	healthpb.RegisterHealthServer(grpcServer, healthSrv)
	reflection.Register(grpcServer)

	lis, err := net.Listen("tcp", ":"+cfg.GRPCPort)
	if err != nil {
		log.Error("не удалось открыть порт", "port", cfg.GRPCPort, "error", err)
		os.Exit(1)
	}

	go func() {
		<-ctx.Done()
		log.Info("останавливаю booking-svc")
		grpcServer.GracefulStop()
	}()

	log.Info("booking-svc запущен", "grpc_port", cfg.GRPCPort)
	if err := grpcServer.Serve(lis); err != nil && !errors.Is(err, grpc.ErrServerStopped) {
		log.Error("ошибка gRPC-сервера", "error", err)
		os.Exit(1)
	}
}
