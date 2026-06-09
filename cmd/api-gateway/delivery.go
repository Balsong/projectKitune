package main

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	deliveryv1 "tea-platform/internal/genpb/delivery/v1"
	"tea-platform/pkg/response"
)

type deliveryDTO struct {
	OrderID         string `json:"order_id"`
	FulfillmentType string `json:"fulfillment_type"`
	Status          string `json:"status"`
	TrackingCode    string `json:"tracking_code"`
	Courier         string `json:"courier"`
	ETA             string `json:"eta"`
	CreatedAt       string `json:"created_at"`
}

// handleGetDelivery возвращает статус доставки по заказу (/api/v1/orders/{id}/delivery).
func handleGetDelivery(log *slog.Logger, client deliveryv1.DeliveryServiceClient) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()

		d, err := client.GetDelivery(ctx, &deliveryv1.GetDeliveryRequest{OrderId: r.PathValue("id")})
		if err != nil {
			writeGRPCError(w, log, "GetDelivery", err)
			return
		}
		response.WriteJSON(w, http.StatusOK, deliveryDTO{
			OrderID:         d.GetOrderId(),
			FulfillmentType: d.GetFulfillmentType(),
			Status:          d.GetStatus().String(),
			TrackingCode:    d.GetTrackingCode(),
			Courier:         d.GetCourier(),
			ETA:             d.GetEta(),
			CreatedAt:       d.GetCreatedAt(),
		})
	}
}
