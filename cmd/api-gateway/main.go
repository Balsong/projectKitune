// Command api-gateway — единая точка входа (BFF): принимает REST/JSON снаружи
// и проксирует во внутренние сервисы по gRPC. Бизнес-логики не содержит.
package main

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"os"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	"tea-platform/internal/config"
	catalogv1 "tea-platform/internal/genpb/catalog/v1"
	"tea-platform/internal/kafka"
	"tea-platform/pkg/response"
)

func main() {
	log := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	cfg := config.Load()

	// gRPC-клиент к Catalog Service.
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

	// Kafka producer (бронь столов).
	kafkaProd, err := kafka.NewProducer(cfg.KafkaBrokers)
	if err != nil {
		log.Error("не удалось инициализировать Kafka", "error", err)
		os.Exit(1)
	}
	defer func() { _ = kafkaProd.Close() }()

	mux := http.NewServeMux()
	mux.HandleFunc("/health", healthHandler)
	mux.HandleFunc("/api/v1/menu", handleGetMenu(log, catalogClient))
	mux.HandleFunc("/api/v1/book", handleBookTable(log, kafkaProd))

	srv := &http.Server{
		Addr:              ":" + cfg.Port,
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
	}

	log.Info("api-gateway запущен", "addr", srv.Addr, "catalog", cfg.CatalogAddr)
	if err := srv.ListenAndServe(); err != nil {
		log.Error("ошибка HTTP-сервера", "error", err)
		os.Exit(1)
	}
}

func healthHandler(w http.ResponseWriter, _ *http.Request) {
	response.WriteJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// menuItemDTO — представление позиции каталога для фронтенда.
type menuItemDTO struct {
	ID         string `json:"id"`
	Kind       string `json:"kind"`
	Name       string `json:"name"`
	Category   string `json:"category"`
	PriceCents int64  `json:"price_cents"`
	Currency   string `json:"currency"`
	Available  bool   `json:"available"`
	ImageURL   string `json:"image_url"`
}

// handleGetMenu проксирует запрос меню в Catalog Service по gRPC.
func handleGetMenu(log *slog.Logger, client catalogv1.CatalogServiceClient) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			response.Error(w, http.StatusMethodNotAllowed, "Method not allowed")
			return
		}

		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()

		resp, err := client.ListProducts(ctx, &catalogv1.ListProductsRequest{})
		if err != nil {
			log.Error("catalog ListProducts failed", "error", err)
			response.Error(w, http.StatusBadGateway, "Catalog service unavailable")
			return
		}

		items := make([]menuItemDTO, 0, len(resp.GetProducts()))
		for _, p := range resp.GetProducts() {
			items = append(items, menuItemDTO{
				ID:         p.GetId(),
				Kind:       p.GetKind().String(),
				Name:       p.GetName(),
				Category:   p.GetCategory(),
				PriceCents: p.GetPriceCents(),
				Currency:   p.GetCurrency(),
				Available:  p.GetAvailable(),
				ImageURL:   p.GetImageUrl(),
			})
		}
		response.WriteJSON(w, http.StatusOK, items)
	}
}

// handleBookTable принимает бронь и публикует её в Kafka.
func handleBookTable(log *slog.Logger, producer *kafka.Producer) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			response.Error(w, http.StatusMethodNotAllowed, "Method not allowed")
			return
		}

		var booking struct {
			TableID  int    `json:"table_id"`
			Customer string `json:"customer"`
			Time     string `json:"time"`
		}
		if err := json.NewDecoder(r.Body).Decode(&booking); err != nil {
			response.Error(w, http.StatusBadRequest, "Invalid request body")
			return
		}

		if err := producer.SendBooking(booking); err != nil {
			log.Error("не удалось отправить бронь в Kafka", "error", err)
			response.Error(w, http.StatusInternalServerError, "Failed to process booking")
			return
		}

		response.WriteJSON(w, http.StatusAccepted, map[string]string{
			"status":  "booking_pending",
			"message": "Your booking is being processed",
		})
	}
}
