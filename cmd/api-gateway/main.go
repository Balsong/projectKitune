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
	cartv1 "tea-platform/internal/genpb/cart/v1"
	catalogv1 "tea-platform/internal/genpb/catalog/v1"
	deliveryv1 "tea-platform/internal/genpb/delivery/v1"
	orderv1 "tea-platform/internal/genpb/order/v1"
	"tea-platform/internal/kafka"
	"tea-platform/internal/metrics"
	"tea-platform/pkg/events"
	"tea-platform/pkg/response"
)

func main() {
	log := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	cfg := config.Load()

	go metrics.Serve(":"+cfg.MetricsPort, log)

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

	// gRPC-клиент к Cart Service.
	cartConn, err := grpc.NewClient(
		cfg.CartAddr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		log.Error("не удалось создать gRPC-клиент Cart", "error", err)
		os.Exit(1)
	}
	defer func() { _ = cartConn.Close() }()
	cartClient := cartv1.NewCartServiceClient(cartConn)

	// gRPC-клиент к Order Service.
	orderConn, err := grpc.NewClient(
		cfg.OrderAddr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		log.Error("не удалось создать gRPC-клиент Order", "error", err)
		os.Exit(1)
	}
	defer func() { _ = orderConn.Close() }()
	orderClient := orderv1.NewOrderServiceClient(orderConn)

	// gRPC-клиент к Delivery Service.
	deliveryConn, err := grpc.NewClient(
		cfg.DeliveryAddr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		log.Error("не удалось создать gRPC-клиент Delivery", "error", err)
		os.Exit(1)
	}
	defer func() { _ = deliveryConn.Close() }()
	deliveryClient := deliveryv1.NewDeliveryServiceClient(deliveryConn)

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

	// Корзина (метод-специфичные маршруты, Go 1.22+ ServeMux).
	mux.HandleFunc("GET /api/v1/cart", handleGetCart(log, cartClient))
	mux.HandleFunc("POST /api/v1/cart/items", handleAddItem(log, cartClient))
	mux.HandleFunc("DELETE /api/v1/cart/items", handleRemoveItem(log, cartClient))
	mux.HandleFunc("POST /api/v1/cart/clear", handleClearCart(log, cartClient))
	mux.HandleFunc("POST /api/v1/cart/merge", handleMergeCart(log, cartClient))

	// Заказы.
	mux.HandleFunc("POST /api/v1/orders", handleCreateOrder(log, orderClient))
	mux.HandleFunc("GET /api/v1/orders/{id}", handleGetOrder(log, orderClient))
	mux.HandleFunc("GET /api/v1/orders/{id}/delivery", handleGetDelivery(log, deliveryClient))

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
	ID          string `json:"id"`
	SKU         string `json:"sku"`
	Kind        string `json:"kind"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Category    string `json:"category"`
	PriceCents  int64  `json:"price_cents"`
	Currency    string `json:"currency"`
	Available   bool   `json:"available"`
	ImageURL    string `json:"image_url"`
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
				ID:          p.GetId(),
				SKU:         p.GetSku(),
				Kind:        p.GetKind().String(),
				Name:        p.GetName(),
				Description: p.GetDescription(),
				Category:    p.GetCategory(),
				PriceCents:  p.GetPriceCents(),
				Currency:    p.GetCurrency(),
				Available:   p.GetAvailable(),
				ImageURL:    p.GetImageUrl(),
			})
		}
		response.WriteJSON(w, http.StatusOK, items)
	}
}

// bookingRequestedPayload — полезная нагрузка события booking.requested.
type bookingRequestedPayload struct {
	BookingID string `json:"booking_id"`
	TableID   int    `json:"table_id"`
	Customer  string `json:"customer"`
	Time      string `json:"time"`
}

// handleBookTable принимает бронь и публикует событие booking.requested в Kafka.
func handleBookTable(log *slog.Logger, producer *kafka.Producer) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			response.Error(w, http.StatusMethodNotAllowed, "Method not allowed")
			return
		}

		var body struct {
			TableID  int    `json:"table_id"`
			Customer string `json:"customer"`
			Time     string `json:"time"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			response.Error(w, http.StatusBadRequest, "Invalid request body")
			return
		}

		bookingID := events.NewID()
		env, err := events.New(events.EventBookingRequested, 1, bookingID, bookingRequestedPayload{
			BookingID: bookingID,
			TableID:   body.TableID,
			Customer:  body.Customer,
			Time:      body.Time,
		})
		if err != nil {
			log.Error("не удалось собрать событие брони", "error", err)
			response.Error(w, http.StatusInternalServerError, "Failed to process booking")
			return
		}

		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()
		if err := producer.Publish(ctx, events.TopicBookings, env); err != nil {
			log.Error("не удалось опубликовать бронь в Kafka", "error", err)
			response.Error(w, http.StatusInternalServerError, "Failed to process booking")
			return
		}

		response.WriteJSON(w, http.StatusAccepted, map[string]string{
			"status":     "booking_pending",
			"booking_id": bookingID,
			"message":    "Your booking is being processed",
		})
	}
}
