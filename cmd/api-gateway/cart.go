package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	cartv1 "tea-platform/internal/genpb/cart/v1"
	"tea-platform/pkg/response"
)

// cartIDHeader — заголовок, в котором клиент передаёт/получает идентификатор
// корзины (session_id гостя или user_id). Если пуст — генерируем новый.
const cartIDHeader = "X-Cart-Id"

// cartItemDTO/cartDTO — представление корзины для фронтенда.
type cartItemDTO struct {
	ProductID      string `json:"product_id"`
	Quantity       int32  `json:"quantity"`
	Name           string `json:"name"`
	UnitPriceCents int64  `json:"unit_price_cents"`
	SubtotalCents  int64  `json:"subtotal_cents"`
	Available      bool   `json:"available"`
}

type cartDTO struct {
	CartID     string        `json:"cart_id"`
	Items      []cartItemDTO `json:"items"`
	TotalCents int64         `json:"total_cents"`
	Currency   string        `json:"currency"`
}

// resolveCartID берёт cart_id из заголовка или генерирует новый.
func resolveCartID(r *http.Request) string {
	if id := r.Header.Get(cartIDHeader); id != "" {
		return id
	}
	return newCartID()
}

// newCartID генерирует случайный идентификатор корзины.
func newCartID() string {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		// crypto/rand практически не падает; на всякий случай — временная метка.
		return "cart-" + time.Now().Format("20060102150405.000000000")
	}
	return hex.EncodeToString(buf)
}

// writeCart отдаёт корзину клиенту, дублируя cart_id в заголовок ответа.
func writeCart(w http.ResponseWriter, status int, cart *cartv1.Cart) {
	items := make([]cartItemDTO, 0, len(cart.GetItems()))
	for _, it := range cart.GetItems() {
		items = append(items, cartItemDTO{
			ProductID:      it.GetProductId(),
			Quantity:       it.GetQuantity(),
			Name:           it.GetName(),
			UnitPriceCents: it.GetUnitPriceCents(),
			SubtotalCents:  it.GetSubtotalCents(),
			Available:      it.GetAvailable(),
		})
	}
	w.Header().Set(cartIDHeader, cart.GetCartId())
	response.WriteJSON(w, status, cartDTO{
		CartID:     cart.GetCartId(),
		Items:      items,
		TotalCents: cart.GetTotalCents(),
		Currency:   cart.GetCurrency(),
	})
}

// handleGetCart возвращает текущую корзину.
func handleGetCart(log *slog.Logger, client cartv1.CartServiceClient) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()

		cart, err := client.GetCart(ctx, &cartv1.GetCartRequest{CartId: resolveCartID(r)})
		if err != nil {
			log.Error("cart GetCart failed", "error", err)
			response.Error(w, http.StatusBadGateway, "Cart service unavailable")
			return
		}
		writeCart(w, http.StatusOK, cart)
	}
}

// handleAddItem добавляет позицию в корзину.
func handleAddItem(log *slog.Logger, client cartv1.CartServiceClient) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			ProductID string `json:"product_id"`
			Quantity  int32  `json:"quantity"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			response.Error(w, http.StatusBadRequest, "Invalid request body")
			return
		}

		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()

		cart, err := client.AddItem(ctx, &cartv1.AddItemRequest{
			CartId:    resolveCartID(r),
			ProductId: body.ProductID,
			Quantity:  body.Quantity,
		})
		if err != nil {
			writeGRPCError(w, log, "AddItem", err)
			return
		}
		writeCart(w, http.StatusOK, cart)
	}
}

// handleRemoveItem уменьшает количество или удаляет позицию.
func handleRemoveItem(log *slog.Logger, client cartv1.CartServiceClient) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			ProductID string `json:"product_id"`
			Quantity  int32  `json:"quantity"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			response.Error(w, http.StatusBadRequest, "Invalid request body")
			return
		}

		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()

		cart, err := client.RemoveItem(ctx, &cartv1.RemoveItemRequest{
			CartId:    resolveCartID(r),
			ProductId: body.ProductID,
			Quantity:  body.Quantity,
		})
		if err != nil {
			writeGRPCError(w, log, "RemoveItem", err)
			return
		}
		writeCart(w, http.StatusOK, cart)
	}
}

// handleClearCart очищает корзину.
func handleClearCart(log *slog.Logger, client cartv1.CartServiceClient) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()

		cart, err := client.ClearCart(ctx, &cartv1.ClearCartRequest{CartId: resolveCartID(r)})
		if err != nil {
			log.Error("cart ClearCart failed", "error", err)
			response.Error(w, http.StatusBadGateway, "Cart service unavailable")
			return
		}
		writeCart(w, http.StatusOK, cart)
	}
}

// handleMergeCart переносит гостевую корзину в текущую (при логине).
func handleMergeCart(log *slog.Logger, client cartv1.CartServiceClient) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			SourceCartID string `json:"source_cart_id"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			response.Error(w, http.StatusBadRequest, "Invalid request body")
			return
		}

		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()

		cart, err := client.MergeCart(ctx, &cartv1.MergeCartRequest{
			SourceCartId: body.SourceCartID,
			TargetCartId: resolveCartID(r),
		})
		if err != nil {
			writeGRPCError(w, log, "MergeCart", err)
			return
		}
		writeCart(w, http.StatusOK, cart)
	}
}
