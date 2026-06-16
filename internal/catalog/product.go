// Package catalog содержит доменную модель и логику сервиса каталога:
// репозиторий на pgx и gRPC-сервер с read-through кэшем в Redis.
package catalog

import (
	"errors"

	catalogv1 "tea-platform/internal/genpb/catalog/v1"
)

// ErrNotFound возвращается, когда товар/блюдо не найдены.
var ErrNotFound = errors.New("catalog: product not found")

// Виды позиций каталога (хранятся в колонке kind).
const (
	KindTeaGoods = "tea_goods"
	KindDish     = "dish"
)

// Product — доменная модель позиции каталога. Не зависит от транспорта.
type Product struct {
	ID          string `json:"id"`
	Kind        string `json:"kind"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Category    string `json:"category"`
	PriceCents  int64  `json:"price_cents"`
	Currency    string `json:"currency"`
	Available   bool   `json:"available"`
	ImageURL    string `json:"image_url"`
	SKU         string `json:"sku"`
	Unit        string `json:"unit"`
}

// toProto конвертирует доменную модель в protobuf-сообщение.
func (p Product) toProto() *catalogv1.Product {
	return &catalogv1.Product{
		Id:          p.ID,
		Kind:        kindToProto(p.Kind),
		Name:        p.Name,
		Description: p.Description,
		Category:    p.Category,
		PriceCents:  p.PriceCents,
		Currency:    p.Currency,
		Available:   p.Available,
		ImageUrl:    p.ImageURL,
		Sku:         p.SKU,
		Unit:        p.Unit,
	}
}

// kindToProto переводит строковый вид в enum protobuf.
func kindToProto(kind string) catalogv1.ProductKind {
	switch kind {
	case KindTeaGoods:
		return catalogv1.ProductKind_PRODUCT_KIND_TEA_GOODS
	case KindDish:
		return catalogv1.ProductKind_PRODUCT_KIND_DISH
	default:
		return catalogv1.ProductKind_PRODUCT_KIND_UNSPECIFIED
	}
}

// kindFromProto переводит enum protobuf в строковый вид; UNSPECIFIED → "".
func kindFromProto(kind catalogv1.ProductKind) string {
	switch kind {
	case catalogv1.ProductKind_PRODUCT_KIND_TEA_GOODS:
		return KindTeaGoods
	case catalogv1.ProductKind_PRODUCT_KIND_DISH:
		return KindDish
	default:
		return ""
	}
}
