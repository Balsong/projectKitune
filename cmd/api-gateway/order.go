package main

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	orderv1 "tea-platform/internal/genpb/order/v1"
	"tea-platform/pkg/response"
)

// fulfillmentByName сопоставляет строковый способ получения с enum proto.
var fulfillmentByName = map[string]orderv1.FulfillmentType{
	"shop_delivery": orderv1.FulfillmentType_FULFILLMENT_TYPE_SHOP_DELIVERY,
	"food_courier":  orderv1.FulfillmentType_FULFILLMENT_TYPE_FOOD_COURIER,
	"dine_in":       orderv1.FulfillmentType_FULFILLMENT_TYPE_DINE_IN,
}

type orderItemDTO struct {
	ProductID      string `json:"product_id"`
	Name           string `json:"name"`
	Quantity       int32  `json:"quantity"`
	UnitPriceCents int64  `json:"unit_price_cents"`
	SubtotalCents  int64  `json:"subtotal_cents"`
}

type orderDTO struct {
	ID              string         `json:"id"`
	UserID          string         `json:"user_id"`
	FulfillmentType string         `json:"fulfillment_type"`
	Status          string         `json:"status"`
	Items           []orderItemDTO `json:"items"`
	TotalCents      int64          `json:"total_cents"`
	Currency        string         `json:"currency"`
	Address         string         `json:"address"`
	CreatedAt       string         `json:"created_at"`
}

func toOrderDTO(o *orderv1.Order) orderDTO {
	items := make([]orderItemDTO, 0, len(o.GetItems()))
	for _, it := range o.GetItems() {
		items = append(items, orderItemDTO{
			ProductID:      it.GetProductId(),
			Name:           it.GetName(),
			Quantity:       it.GetQuantity(),
			UnitPriceCents: it.GetUnitPriceCents(),
			SubtotalCents:  it.GetSubtotalCents(),
		})
	}
	return orderDTO{
		ID:              o.GetId(),
		UserID:          o.GetUserId(),
		FulfillmentType: o.GetFulfillmentType().String(),
		Status:          o.GetStatus().String(),
		Items:           items,
		TotalCents:      o.GetTotalCents(),
		Currency:        o.GetCurrency(),
		Address:         o.GetAddress(),
		CreatedAt:       o.GetCreatedAt(),
	}
}

// handleCreateOrder оформляет заказ из текущей корзины (по X-Cart-Id).
func handleCreateOrder(log *slog.Logger, client orderv1.OrderServiceClient) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			FulfillmentType string `json:"fulfillment_type"`
			Address         string `json:"address"`
			UserID          string `json:"user_id"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			response.Error(w, http.StatusBadRequest, "Invalid request body")
			return
		}

		ft, ok := fulfillmentByName[body.FulfillmentType]
		if !ok {
			response.Error(w, http.StatusBadRequest,
				"fulfillment_type must be one of: shop_delivery, food_courier, dine_in")
			return
		}

		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()

		ord, err := client.CreateOrder(ctx, &orderv1.CreateOrderRequest{
			CartId:          resolveCartID(r),
			UserId:          body.UserID,
			FulfillmentType: ft,
			Address:         body.Address,
		})
		if err != nil {
			writeGRPCError(w, log, "CreateOrder", err)
			return
		}
		response.WriteJSON(w, http.StatusCreated, toOrderDTO(ord))
	}
}

// handleGetOrder возвращает заказ по id из пути (/api/v1/orders/{id}).
func handleGetOrder(log *slog.Logger, client orderv1.OrderServiceClient) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()

		ord, err := client.GetOrder(ctx, &orderv1.GetOrderRequest{Id: r.PathValue("id")})
		if err != nil {
			writeGRPCError(w, log, "GetOrder", err)
			return
		}
		response.WriteJSON(w, http.StatusOK, toOrderDTO(ord))
	}
}
