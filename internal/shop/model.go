// Package shop реализует интернет-магазин чая: каталог с реальным складским
// остатком, изолированная корзина, оформление заказа с доставкой по стране.
package shop

import (
	"time"

	shopv1 "tea-platform/internal/genpb/shop/v1"
)

// Product — товар магазина.
type Product struct {
	ID          string
	SKU         string
	Name        string
	Description string
	Category    string
	PriceCents  int64
	Currency    string
	WeightGrams int32
	Unit        string
	ImageURL    string
	StockQty    int32
	Available   bool
}

func (p Product) toProto() *shopv1.Product {
	return &shopv1.Product{
		Id: p.ID, Sku: p.SKU, Name: p.Name, Description: p.Description,
		Category: p.Category, PriceCents: p.PriceCents, Currency: p.Currency,
		WeightGrams: p.WeightGrams, Unit: p.Unit, ImageUrl: p.ImageURL,
		StockQty: p.StockQty, Available: p.Available,
	}
}

// CartLine — позиция корзины с подтянутыми данными товара.
type CartLine struct {
	ProductID      string
	SKU            string
	Name           string
	Quantity       int32
	UnitPriceCents int64
	SubtotalCents  int64
	StockQty       int32
}

func (l CartLine) toProto() *shopv1.CartItem {
	return &shopv1.CartItem{
		ProductId: l.ProductID, Sku: l.SKU, Name: l.Name, Quantity: l.Quantity,
		UnitPriceCents: l.UnitPriceCents, SubtotalCents: l.SubtotalCents, StockQty: l.StockQty,
	}
}

// OrderLine — позиция заказа (снимок на момент покупки).
type OrderLine struct {
	ProductID      string
	Name           string
	Quantity       int32
	UnitPriceCents int64
	SubtotalCents  int64
}

func (l OrderLine) toProto() *shopv1.OrderItem {
	return &shopv1.OrderItem{
		ProductId: l.ProductID, Name: l.Name, Quantity: l.Quantity,
		UnitPriceCents: l.UnitPriceCents, SubtotalCents: l.SubtotalCents,
	}
}

// Order — заказ магазина.
type Order struct {
	ID                 string
	UserID             string
	Status             string
	Customer           string
	Phone              string
	Email              string
	Country            string
	Region             string
	City               string
	Address            string
	PostalCode         string
	Comment            string
	ItemsSubtotalCents int64
	DeliveryCents      int64
	TotalCents         int64
	TrackingCode       string
	Items              []OrderLine
	CreatedAt          time.Time
}

func (o Order) toProto() *shopv1.Order {
	items := make([]*shopv1.OrderItem, 0, len(o.Items))
	for _, it := range o.Items {
		items = append(items, it.toProto())
	}
	return &shopv1.Order{
		Id: o.ID, UserId: o.UserID, Status: o.Status, Customer: o.Customer,
		Phone: o.Phone, Email: o.Email, Country: o.Country, Region: o.Region,
		City: o.City, Address: o.Address, PostalCode: o.PostalCode, Comment: o.Comment,
		ItemsSubtotalCents: o.ItemsSubtotalCents, DeliveryCents: o.DeliveryCents,
		TotalCents: o.TotalCents, TrackingCode: o.TrackingCode, Items: items,
		CreatedAt: o.CreatedAt.UTC().Format(time.RFC3339),
	}
}
