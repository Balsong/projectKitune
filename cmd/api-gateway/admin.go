package main

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	accountv1 "tea-platform/internal/genpb/account/v1"
	bookingv1 "tea-platform/internal/genpb/booking/v1"
	catalogv1 "tea-platform/internal/genpb/catalog/v1"
	orderv1 "tea-platform/internal/genpb/order/v1"
	"tea-platform/pkg/response"
)

// requireAdmin проверяет сессию и роль admin. Возвращает false и пишет ответ
// (401/403), если доступ запрещён.
func requireAdmin(ctx context.Context, account accountv1.AccountServiceClient, w http.ResponseWriter, r *http.Request) bool {
	token := bearerToken(r)
	if token == "" {
		response.Error(w, http.StatusUnauthorized, "Требуется вход")
		return false
	}
	u, err := account.GetSession(ctx, &accountv1.SessionRequest{SessionToken: token})
	if err != nil {
		response.Error(w, http.StatusUnauthorized, "Сессия недействительна")
		return false
	}
	if u.GetRole() != "admin" {
		response.Error(w, http.StatusForbidden, "Доступ только для администратора")
		return false
	}
	return true
}

// handleAdminUpdateProduct меняет цену и доступность позиции меню.
func handleAdminUpdateProduct(log *slog.Logger, account accountv1.AccountServiceClient, client catalogv1.CatalogServiceClient) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()
		if !requireAdmin(ctx, account, w, r) {
			return
		}

		var body struct {
			PriceCents int64 `json:"price_cents"`
			Available  bool  `json:"available"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			response.Error(w, http.StatusBadRequest, "Invalid request body")
			return
		}

		p, err := client.UpdateProduct(ctx, &catalogv1.UpdateProductRequest{
			Id: r.PathValue("id"), PriceCents: body.PriceCents, Available: body.Available,
		})
		if err != nil {
			writeGRPCError(w, log, "UpdateProduct", err)
			return
		}
		response.WriteJSON(w, http.StatusOK, menuItemDTO{
			ID: p.GetId(), SKU: p.GetSku(), Kind: p.GetKind().String(), Name: p.GetName(),
			Description: p.GetDescription(), Category: p.GetCategory(), PriceCents: p.GetPriceCents(),
			Currency: p.GetCurrency(), Available: p.GetAvailable(), ImageURL: p.GetImageUrl(), Unit: p.GetUnit(),
		})
	}
}

// adminOrderDTO — заказ для админ-таблицы.
type adminOrderDTO struct {
	ID              string `json:"id"`
	UserID          string `json:"user_id"`
	Status          string `json:"status"`
	FulfillmentType string `json:"fulfillment_type"`
	TotalCents      int64  `json:"total_cents"`
	Address         string `json:"address"`
	CreatedAt       string `json:"created_at"`
}

// handleAdminOrders возвращает все заказы (админка).
func handleAdminOrders(log *slog.Logger, account accountv1.AccountServiceClient, client orderv1.OrderServiceClient) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()
		if !requireAdmin(ctx, account, w, r) {
			return
		}

		resp, err := client.ListAllOrders(ctx, &orderv1.ListAllOrdersRequest{})
		if err != nil {
			writeGRPCError(w, log, "ListAllOrders", err)
			return
		}
		out := make([]adminOrderDTO, 0, len(resp.GetOrders()))
		for _, o := range resp.GetOrders() {
			out = append(out, adminOrderDTO{
				ID: o.GetId(), UserID: o.GetUserId(), Status: o.GetStatus().String(),
				FulfillmentType: o.GetFulfillmentType().String(), TotalCents: o.GetTotalCents(),
				Address: o.GetAddress(), CreatedAt: o.GetCreatedAt(),
			})
		}
		response.WriteJSON(w, http.StatusOK, out)
	}
}

// handleAdminBookings возвращает все брони (админка).
func handleAdminBookings(log *slog.Logger, account accountv1.AccountServiceClient, client bookingv1.BookingServiceClient) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()
		if !requireAdmin(ctx, account, w, r) {
			return
		}

		resp, err := client.ListAllBookings(ctx, &bookingv1.ListAllBookingsRequest{})
		if err != nil {
			writeGRPCError(w, log, "ListAllBookings", err)
			return
		}
		out := make([]bookingDTO, 0, len(resp.GetBookings()))
		for _, b := range resp.GetBookings() {
			out = append(out, toBookingDTO(b))
		}
		response.WriteJSON(w, http.StatusOK, out)
	}
}

// handleAdminUpdateBooking меняет статус брони (админка).
func handleAdminUpdateBooking(log *slog.Logger, account accountv1.AccountServiceClient, client bookingv1.BookingServiceClient) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()
		if !requireAdmin(ctx, account, w, r) {
			return
		}

		var body struct {
			Status string `json:"status"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			response.Error(w, http.StatusBadRequest, "Invalid request body")
			return
		}

		b, err := client.UpdateBookingStatus(ctx, &bookingv1.UpdateBookingStatusRequest{
			Id: r.PathValue("id"), Status: body.Status,
		})
		if err != nil {
			writeGRPCError(w, log, "UpdateBookingStatus", err)
			return
		}
		response.WriteJSON(w, http.StatusOK, toBookingDTO(b))
	}
}
