// Package delivery реализует Delivery Service: по order.confirmed создаёт
// доставку (трек/курьер в зависимости от способа получения), фоновый симулятор
// доводит её до delivered. gRPC отдаёт статус доставки.
package delivery

import (
	"errors"
	"time"

	deliveryv1 "tea-platform/internal/genpb/delivery/v1"
)

// ErrNotFound возвращается, когда доставка по заказу не найдена.
var ErrNotFound = errors.New("delivery: not found")

// Статусы доставки.
const (
	StatusCreated    = "created"
	StatusDispatched = "dispatched"
	StatusDelivered  = "delivered"
)

// Delivery — доменная модель доставки.
type Delivery struct {
	OrderID         string
	FulfillmentType string
	Status          string
	TrackingCode    string
	Courier         string
	ETA             string
	CorrelationID   string
	CreatedAt       time.Time
}

func (d *Delivery) toProto() *deliveryv1.Delivery {
	return &deliveryv1.Delivery{
		OrderId:         d.OrderID,
		FulfillmentType: d.FulfillmentType,
		Status:          statusToProto(d.Status),
		TrackingCode:    d.TrackingCode,
		Courier:         d.Courier,
		Eta:             d.ETA,
		CreatedAt:       d.CreatedAt.UTC().Format(time.RFC3339),
	}
}

func statusToProto(s string) deliveryv1.DeliveryStatus {
	switch s {
	case StatusCreated:
		return deliveryv1.DeliveryStatus_DELIVERY_STATUS_CREATED
	case StatusDispatched:
		return deliveryv1.DeliveryStatus_DELIVERY_STATUS_DISPATCHED
	case StatusDelivered:
		return deliveryv1.DeliveryStatus_DELIVERY_STATUS_DELIVERED
	default:
		return deliveryv1.DeliveryStatus_DELIVERY_STATUS_UNSPECIFIED
	}
}
