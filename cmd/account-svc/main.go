// Command account-svc — gRPC-сервис аккаунтов: регистрация, вход, сессии.
// Пользователи в PostgreSQL (bcrypt), сессии в Redis.
package main

import (
	"context"
	"errors"
	"log/slog"
	"net"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"google.golang.org/grpc"
	"google.golang.org/grpc/health"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/reflection"

	"tea-platform/internal/account"
	"tea-platform/internal/cache"
	"tea-platform/internal/config"
	"tea-platform/internal/db"
	accountv1 "tea-platform/internal/genpb/account/v1"
	"tea-platform/internal/metrics"
	"tea-platform/migrations"
)

func main() {
	log := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	cfg := config.Load()

	go metrics.Serve(":"+cfg.MetricsPort, log)

	ctx := context.Background()

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

	rdb, err := cache.NewRedis(ctx, cfg.RedisAddr)
	if err != nil {
		log.Error("не удалось подключиться к Redis", "error", err)
		os.Exit(1)
	}
	defer func() { _ = rdb.Close() }()

	var adminEmails []string
	if cfg.AdminEmails != "" {
		adminEmails = strings.Split(cfg.AdminEmails, ",")
	}
	svc := account.NewService(account.NewRepository(pool), rdb, log, adminEmails)

	// Бутстрап: промоутим уже зарегистрированных админов из ADMIN_EMAILS.
	if len(adminEmails) > 0 {
		if err := svc.PromoteAdmins(ctx, adminEmails); err != nil {
			log.Warn("не удалось назначить админов", "error", err)
		} else {
			log.Info("назначены админы из ADMIN_EMAILS", "emails", cfg.AdminEmails)
		}
	}

	grpcServer := grpc.NewServer()
	accountv1.RegisterAccountServiceServer(grpcServer, svc)

	healthSrv := health.NewServer()
	healthSrv.SetServingStatus("account.v1.AccountService", healthpb.HealthCheckResponse_SERVING)
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
		log.Info("останавливаю account-svc")
		grpcServer.GracefulStop()
	}()

	log.Info("account-svc запущен", "grpc_port", cfg.GRPCPort)
	if err := grpcServer.Serve(lis); err != nil && !errors.Is(err, grpc.ErrServerStopped) {
		log.Error("ошибка gRPC-сервера", "error", err)
		os.Exit(1)
	}
}
