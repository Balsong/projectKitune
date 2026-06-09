// Package order содержит доменную модель и логику Order Service: репозиторий
// (создание заказа в одной транзакции с outbox) и gRPC-сервер.
package order

import (
	"errors"
	"time"

	orderv1 "tea-platform/internal/genpb/order/v1"
)

// ErrNotFound возвращается, когда заказ не найден.
var ErrNotFound = errors.New("order: not found")

// Статусы заказа (колонка status).
const (
	StatusCreated        = "created"
	StatusPaymentPending = "payment_pending"
	StatusPaid           = "paid"
	StatusConfirmed      = "confirmed"
	StatusCompleted      = "completed"
	StatusCancelled      = "cancelled"
	StatusPaymentFailed  = "payment_failed"
)

// Способы получения (колонка fulfillment_type).
const (
	FulfillmentShopDelivery = "shop_delivery"
	FulfillmentFoodCourier  = "food_courier"
	FulfillmentDineIn       = "dine_in"
)

// Item — позиция заказа. JSON-теги используются в payload события order.created.
type Item struct {
	ProductID      string `json:"product_id"`
	Name           string `json:"name"`
	Quantity       int32  `json:"quantity"`
	UnitPriceCents int64  `json:"unit_price_cents"`
	SubtotalCents  int64  `json:"subtotal_cents"`
}

// Order — доменная модель заказа.
type Order struct {
	ID              string
	UserID          string
	CartID          string
	FulfillmentType string
	Status          string
	Items           []Item
	TotalCents      int64
	Currency        string
	Address         string
	BookingID       string
	CreatedAt       time.Time
}

// OrderCreatedPayload — полезная нагрузка события order.created.
type OrderCreatedPayload struct {
	OrderID         string `json:"order_id"`
	UserID          string `json:"user_id"`
	FulfillmentType string `json:"fulfillment_type"`
	Items           []Item `json:"items"`
	TotalCents      int64  `json:"total_cents"`
	Currency        string `json:"currency"`
}

// toProto конвертирует заказ в protobuf-сообщение.
func (o *Order) toProto() *orderv1.Order {
	items := make([]*orderv1.OrderItem, 0, len(o.Items))
	for _, it := range o.Items {
		items = append(items, &orderv1.OrderItem{
			ProductId:      it.ProductID,
			Name:           it.Name,
			Quantity:       it.Quantity,
			UnitPriceCents: it.UnitPriceCents,
			SubtotalCents:  it.SubtotalCents,
		})
	}
	return &orderv1.Order{
		Id:              o.ID,
		UserId:          o.UserID,
		CartId:          o.CartID,
		FulfillmentType: fulfillmentToProto(o.FulfillmentType),
		Status:          statusToProto(o.Status),
		Items:           items,
		TotalCents:      o.TotalCents,
		Currency:        o.Currency,
		Address:         o.Address,
		BookingId:       o.BookingID,
		CreatedAt:       o.CreatedAt.UTC().Format(time.RFC3339),
	}
}

func statusToProto(s string) orderv1.OrderStatus {
	switch s {
	case StatusCreated:
		return orderv1.OrderStatus_ORDER_STATUS_CREATED
	case StatusPaymentPending:
		return orderv1.OrderStatus_ORDER_STATUS_PAYMENT_PENDING
	case StatusPaid:
		return orderv1.OrderStatus_ORDER_STATUS_PAID
	case StatusConfirmed:
		return orderv1.OrderStatus_ORDER_STATUS_CONFIRMED
	case StatusCompleted:
		return orderv1.OrderStatus_ORDER_STATUS_COMPLETED
	case StatusCancelled:
		return orderv1.OrderStatus_ORDER_STATUS_CANCELLED
	case StatusPaymentFailed:
		return orderv1.OrderStatus_ORDER_STATUS_PAYMENT_FAILED
	default:
		return orderv1.OrderStatus_ORDER_STATUS_UNSPECIFIED
	}
}

func fulfillmentToProto(f string) orderv1.FulfillmentType {
	switch f {
	case FulfillmentShopDelivery:
		return orderv1.FulfillmentType_FULFILLMENT_TYPE_SHOP_DELIVERY
	case FulfillmentFoodCourier:
		return orderv1.FulfillmentType_FULFILLMENT_TYPE_FOOD_COURIER
	case FulfillmentDineIn:
		return orderv1.FulfillmentType_FULFILLMENT_TYPE_DINE_IN
	default:
		return orderv1.FulfillmentType_FULFILLMENT_TYPE_UNSPECIFIED
	}
}

// fulfillmentFromProto переводит enum в строковый вид; UNSPECIFIED → "".
func fulfillmentFromProto(f orderv1.FulfillmentType) string {
	switch f {
	case orderv1.FulfillmentType_FULFILLMENT_TYPE_SHOP_DELIVERY:
		return FulfillmentShopDelivery
	case orderv1.FulfillmentType_FULFILLMENT_TYPE_FOOD_COURIER:
		return FulfillmentFoodCourier
	case orderv1.FulfillmentType_FULFILLMENT_TYPE_DINE_IN:
		return FulfillmentDineIn
	default:
		return ""
	}
}
