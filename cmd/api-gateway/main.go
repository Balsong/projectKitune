// Command api-gateway — единая точка входа (BFF): принимает REST/JSON снаружи
// и проксирует во внутренние сервисы по gRPC. Бизнес-логики не содержит.
package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	"tea-platform/internal/config"
	accountv1 "tea-platform/internal/genpb/account/v1"
	bookingv1 "tea-platform/internal/genpb/booking/v1"
	cartv1 "tea-platform/internal/genpb/cart/v1"
	catalogv1 "tea-platform/internal/genpb/catalog/v1"
	deliveryv1 "tea-platform/internal/genpb/delivery/v1"
	orderv1 "tea-platform/internal/genpb/order/v1"
	"tea-platform/internal/metrics"
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

	// gRPC-клиент к Account Service.
	accountConn, err := grpc.NewClient(
		cfg.AccountAddr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		log.Error("не удалось создать gRPC-клиент Account", "error", err)
		os.Exit(1)
	}
	defer func() { _ = accountConn.Close() }()
	accountClient := accountv1.NewAccountServiceClient(accountConn)

	// gRPC-клиент к Booking Service.
	bookingConn, err := grpc.NewClient(
		cfg.BookingAddr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		log.Error("не удалось создать gRPC-клиент Booking", "error", err)
		os.Exit(1)
	}
	defer func() { _ = bookingConn.Close() }()
	bookingClient := bookingv1.NewBookingServiceClient(bookingConn)

	mux := http.NewServeMux()
	mux.HandleFunc("/health", healthHandler)
	mux.HandleFunc("/api/v1/menu", handleGetMenu(log, catalogClient))
	mux.HandleFunc("POST /api/v1/book", handleCreateBooking(log, accountClient, bookingClient))
	mux.HandleFunc("GET /api/v1/bookings", handleMyBookings(log, accountClient, bookingClient))

	// Корзина (метод-специфичные маршруты, Go 1.22+ ServeMux).
	mux.HandleFunc("GET /api/v1/cart", handleGetCart(log, cartClient))
	mux.HandleFunc("POST /api/v1/cart/items", handleAddItem(log, cartClient))
	mux.HandleFunc("DELETE /api/v1/cart/items", handleRemoveItem(log, cartClient))
	mux.HandleFunc("POST /api/v1/cart/clear", handleClearCart(log, cartClient))
	mux.HandleFunc("POST /api/v1/cart/merge", handleMergeCart(log, cartClient))

	// Заказы.
	mux.HandleFunc("POST /api/v1/orders", handleCreateOrder(log, accountClient, orderClient))
	mux.HandleFunc("GET /api/v1/orders", handleMyOrders(log, accountClient, orderClient))
	mux.HandleFunc("GET /api/v1/orders/{id}", handleGetOrder(log, orderClient))
	mux.HandleFunc("GET /api/v1/orders/{id}/delivery", handleGetDelivery(log, deliveryClient))

	// Админка (требует роль admin; проверка внутри хендлеров).
	mux.HandleFunc("GET /api/v1/admin/orders", handleAdminOrders(log, accountClient, orderClient))
	mux.HandleFunc("GET /api/v1/admin/bookings", handleAdminBookings(log, accountClient, bookingClient))
	mux.HandleFunc("POST /api/v1/admin/bookings/{id}/status", handleAdminUpdateBooking(log, accountClient, bookingClient))
	mux.HandleFunc("POST /api/v1/admin/menu/{id}", handleAdminUpdateProduct(log, accountClient, catalogClient))

	// Аккаунт: регистрация, вход, текущий пользователь, выход.
	// Чувствительные маршруты под rate-limit по IP (барьер от брутфорса).
	authLimiter := newRateLimiter(60, time.Minute)
	mux.HandleFunc("POST /api/v1/auth/register", authLimiter.middleware(handleRegister(log, accountClient)))
	mux.HandleFunc("POST /api/v1/auth/login", authLimiter.middleware(handleLogin(log, accountClient)))
	mux.HandleFunc("POST /api/v1/auth/logout", handleLogout(log, accountClient))
	mux.HandleFunc("GET /api/v1/auth/me", handleMe(log, accountClient))
	mux.HandleFunc("POST /api/v1/auth/change-password", authLimiter.middleware(handleChangePassword(log, accountClient)))

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
	Unit        string `json:"unit"`
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
				Unit:        p.GetUnit(),
			})
		}
		response.WriteJSON(w, http.StatusOK, items)
	}
}
