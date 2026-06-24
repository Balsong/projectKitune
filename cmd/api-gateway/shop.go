package main

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	accountv1 "tea-platform/internal/genpb/account/v1"
	shopv1 "tea-platform/internal/genpb/shop/v1"
	"tea-platform/pkg/response"
)

// shopCartIDHeader — идентификатор корзины магазина (изолирован от ресторанной).
const shopCartIDHeader = "X-Shop-Cart-Id"

func shopCartID(r *http.Request) string { return r.Header.Get(shopCartIDHeader) }

type shopProductDTO struct {
	ID          string `json:"id"`
	SKU         string `json:"sku"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Category    string `json:"category"`
	PriceCents  int64  `json:"price_cents"`
	Currency    string `json:"currency"`
	WeightGrams int32  `json:"weight_grams"`
	Unit        string `json:"unit"`
	ImageURL    string `json:"image_url"`
	StockQty    int32  `json:"stock_qty"`
	Available   bool   `json:"available"`
}

func toShopProductDTO(p *shopv1.Product) shopProductDTO {
	return shopProductDTO{
		ID: p.GetId(), SKU: p.GetSku(), Name: p.GetName(), Description: p.GetDescription(),
		Category: p.GetCategory(), PriceCents: p.GetPriceCents(), Currency: p.GetCurrency(),
		WeightGrams: p.GetWeightGrams(), Unit: p.GetUnit(), ImageURL: p.GetImageUrl(),
		StockQty: p.GetStockQty(), Available: p.GetAvailable(),
	}
}

type shopCartItemDTO struct {
	ProductID      string `json:"product_id"`
	SKU            string `json:"sku"`
	Name           string `json:"name"`
	Quantity       int32  `json:"quantity"`
	UnitPriceCents int64  `json:"unit_price_cents"`
	SubtotalCents  int64  `json:"subtotal_cents"`
	StockQty       int32  `json:"stock_qty"`
}

type shopCartDTO struct {
	CartID     string            `json:"cart_id"`
	Items      []shopCartItemDTO `json:"items"`
	TotalCents int64             `json:"total_cents"`
}

func toShopCartDTO(c *shopv1.Cart) shopCartDTO {
	items := make([]shopCartItemDTO, 0, len(c.GetItems()))
	for _, it := range c.GetItems() {
		items = append(items, shopCartItemDTO{
			ProductID: it.GetProductId(), SKU: it.GetSku(), Name: it.GetName(),
			Quantity: it.GetQuantity(), UnitPriceCents: it.GetUnitPriceCents(),
			SubtotalCents: it.GetSubtotalCents(), StockQty: it.GetStockQty(),
		})
	}
	return shopCartDTO{CartID: c.GetCartId(), Items: items, TotalCents: c.GetTotalCents()}
}

type shopOrderItemDTO struct {
	ProductID      string `json:"product_id"`
	Name           string `json:"name"`
	Quantity       int32  `json:"quantity"`
	UnitPriceCents int64  `json:"unit_price_cents"`
	SubtotalCents  int64  `json:"subtotal_cents"`
}

type shopOrderDTO struct {
	ID                 string             `json:"id"`
	UserID             string             `json:"user_id"`
	Status             string             `json:"status"`
	Customer           string             `json:"customer"`
	Phone              string             `json:"phone"`
	Email              string             `json:"email"`
	Country            string             `json:"country"`
	Region             string             `json:"region"`
	City               string             `json:"city"`
	Address            string             `json:"address"`
	PostalCode         string             `json:"postal_code"`
	Comment            string             `json:"comment"`
	ItemsSubtotalCents int64              `json:"items_subtotal_cents"`
	DeliveryCents      int64              `json:"delivery_cents"`
	TotalCents         int64              `json:"total_cents"`
	TrackingCode       string             `json:"tracking_code"`
	Items              []shopOrderItemDTO `json:"items"`
	CreatedAt          string             `json:"created_at"`
}

func toShopOrderDTO(o *shopv1.Order) shopOrderDTO {
	items := make([]shopOrderItemDTO, 0, len(o.GetItems()))
	for _, it := range o.GetItems() {
		items = append(items, shopOrderItemDTO{
			ProductID: it.GetProductId(), Name: it.GetName(), Quantity: it.GetQuantity(),
			UnitPriceCents: it.GetUnitPriceCents(), SubtotalCents: it.GetSubtotalCents(),
		})
	}
	return shopOrderDTO{
		ID: o.GetId(), UserID: o.GetUserId(), Status: o.GetStatus(), Customer: o.GetCustomer(),
		Phone: o.GetPhone(), Email: o.GetEmail(), Country: o.GetCountry(), Region: o.GetRegion(),
		City: o.GetCity(), Address: o.GetAddress(), PostalCode: o.GetPostalCode(), Comment: o.GetComment(),
		ItemsSubtotalCents: o.GetItemsSubtotalCents(), DeliveryCents: o.GetDeliveryCents(),
		TotalCents: o.GetTotalCents(), TrackingCode: o.GetTrackingCode(), Items: items,
		CreatedAt: o.GetCreatedAt(),
	}
}

// ---- Каталог магазина ----

func handleShopMenu(log *slog.Logger, client shopv1.ShopServiceClient) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()
		resp, err := client.ListProducts(ctx, &shopv1.ListProductsRequest{Category: r.URL.Query().Get("category")})
		if err != nil {
			writeGRPCError(w, log, "Shop.ListProducts", err)
			return
		}
		out := make([]shopProductDTO, 0, len(resp.GetProducts()))
		for _, p := range resp.GetProducts() {
			out = append(out, toShopProductDTO(p))
		}
		response.WriteJSON(w, http.StatusOK, out)
	}
}

// ---- Корзина магазина ----

func handleShopCart(log *slog.Logger, client shopv1.ShopServiceClient) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if shopCartID(r) == "" {
			response.WriteJSON(w, http.StatusOK, shopCartDTO{Items: []shopCartItemDTO{}})
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()
		cart, err := client.GetCart(ctx, &shopv1.CartRequest{CartId: shopCartID(r)})
		if err != nil {
			writeGRPCError(w, log, "Shop.GetCart", err)
			return
		}
		response.WriteJSON(w, http.StatusOK, toShopCartDTO(cart))
	}
}

func handleShopAddToCart(log *slog.Logger, client shopv1.ShopServiceClient) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			ProductID string `json:"product_id"`
			Quantity  int32  `json:"quantity"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			response.Error(w, http.StatusBadRequest, "Invalid request body")
			return
		}
		if shopCartID(r) == "" {
			response.Error(w, http.StatusBadRequest, "missing "+shopCartIDHeader)
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()
		cart, err := client.AddToCart(ctx, &shopv1.AddToCartRequest{
			CartId: shopCartID(r), ProductId: body.ProductID, Quantity: body.Quantity,
		})
		if err != nil {
			writeGRPCError(w, log, "Shop.AddToCart", err)
			return
		}
		response.WriteJSON(w, http.StatusOK, toShopCartDTO(cart))
	}
}

func handleShopRemoveFromCart(log *slog.Logger, client shopv1.ShopServiceClient) http.HandlerFunc {
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
		cart, err := client.RemoveFromCart(ctx, &shopv1.RemoveFromCartRequest{
			CartId: shopCartID(r), ProductId: body.ProductID, Quantity: body.Quantity,
		})
		if err != nil {
			writeGRPCError(w, log, "Shop.RemoveFromCart", err)
			return
		}
		response.WriteJSON(w, http.StatusOK, toShopCartDTO(cart))
	}
}

func handleShopClearCart(log *slog.Logger, client shopv1.ShopServiceClient) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()
		cart, err := client.ClearCart(ctx, &shopv1.CartRequest{CartId: shopCartID(r)})
		if err != nil {
			writeGRPCError(w, log, "Shop.ClearCart", err)
			return
		}
		response.WriteJSON(w, http.StatusOK, toShopCartDTO(cart))
	}
}

// ---- Оформление и заказы магазина ----

func handleShopCheckout(log *slog.Logger, account accountv1.AccountServiceClient, client shopv1.ShopServiceClient) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Customer   string `json:"customer"`
			Phone      string `json:"phone"`
			Email      string `json:"email"`
			Country    string `json:"country"`
			Region     string `json:"region"`
			City       string `json:"city"`
			Address    string `json:"address"`
			PostalCode string `json:"postal_code"`
			Comment    string `json:"comment"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			response.Error(w, http.StatusBadRequest, "Invalid request body")
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 8*time.Second)
		defer cancel()
		o, err := client.Checkout(ctx, &shopv1.CheckoutRequest{
			CartId: shopCartID(r), UserId: sessionUserID(ctx, account, r),
			Customer: body.Customer, Phone: body.Phone, Email: body.Email,
			Country: body.Country, Region: body.Region, City: body.City,
			Address: body.Address, PostalCode: body.PostalCode, Comment: body.Comment,
		})
		if err != nil {
			writeGRPCError(w, log, "Shop.Checkout", err)
			return
		}
		response.WriteJSON(w, http.StatusCreated, toShopOrderDTO(o))
	}
}

func handleShopMyOrders(log *slog.Logger, account accountv1.AccountServiceClient, client shopv1.ShopServiceClient) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()
		uid := sessionUserID(ctx, account, r)
		if uid == "" {
			response.Error(w, http.StatusUnauthorized, "Требуется вход")
			return
		}
		resp, err := client.ListOrders(ctx, &shopv1.ListOrdersRequest{UserId: uid})
		if err != nil {
			writeGRPCError(w, log, "Shop.ListOrders", err)
			return
		}
		out := make([]shopOrderDTO, 0, len(resp.GetOrders()))
		for _, o := range resp.GetOrders() {
			out = append(out, toShopOrderDTO(o))
		}
		response.WriteJSON(w, http.StatusOK, out)
	}
}

func handleShopGetOrder(log *slog.Logger, client shopv1.ShopServiceClient) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()
		o, err := client.GetOrder(ctx, &shopv1.GetOrderRequest{Id: r.PathValue("id")})
		if err != nil {
			writeGRPCError(w, log, "Shop.GetOrder", err)
			return
		}
		response.WriteJSON(w, http.StatusOK, toShopOrderDTO(o))
	}
}

// ---- Админка магазина ----

func handleAdminShopProducts(log *slog.Logger, account accountv1.AccountServiceClient, client shopv1.ShopServiceClient) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()
		if !requireAdmin(ctx, account, w, r) {
			return
		}
		resp, err := client.ListProducts(ctx, &shopv1.ListProductsRequest{})
		if err != nil {
			writeGRPCError(w, log, "Shop.ListProducts", err)
			return
		}
		out := make([]shopProductDTO, 0, len(resp.GetProducts()))
		for _, p := range resp.GetProducts() {
			out = append(out, toShopProductDTO(p))
		}
		response.WriteJSON(w, http.StatusOK, out)
	}
}

func handleAdminShopUpdateProduct(log *slog.Logger, account accountv1.AccountServiceClient, client shopv1.ShopServiceClient) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()
		if !requireAdmin(ctx, account, w, r) {
			return
		}
		var body struct {
			PriceCents int64 `json:"price_cents"`
			StockQty   int32 `json:"stock_qty"`
			Available  bool  `json:"available"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			response.Error(w, http.StatusBadRequest, "Invalid request body")
			return
		}
		p, err := client.UpdateProduct(ctx, &shopv1.UpdateProductRequest{
			Id: r.PathValue("id"), PriceCents: body.PriceCents, StockQty: body.StockQty, Available: body.Available,
		})
		if err != nil {
			writeGRPCError(w, log, "Shop.UpdateProduct", err)
			return
		}
		response.WriteJSON(w, http.StatusOK, toShopProductDTO(p))
	}
}

func handleAdminShopOrders(log *slog.Logger, account accountv1.AccountServiceClient, client shopv1.ShopServiceClient) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()
		if !requireAdmin(ctx, account, w, r) {
			return
		}
		resp, err := client.ListAllOrders(ctx, &shopv1.ListAllOrdersRequest{})
		if err != nil {
			writeGRPCError(w, log, "Shop.ListAllOrders", err)
			return
		}
		out := make([]shopOrderDTO, 0, len(resp.GetOrders()))
		for _, o := range resp.GetOrders() {
			out = append(out, toShopOrderDTO(o))
		}
		response.WriteJSON(w, http.StatusOK, out)
	}
}

func handleAdminShopUpdateOrder(log *slog.Logger, account accountv1.AccountServiceClient, client shopv1.ShopServiceClient) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()
		if !requireAdmin(ctx, account, w, r) {
			return
		}
		var body struct {
			Status       string `json:"status"`
			TrackingCode string `json:"tracking_code"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			response.Error(w, http.StatusBadRequest, "Invalid request body")
			return
		}
		o, err := client.UpdateOrderStatus(ctx, &shopv1.UpdateOrderStatusRequest{
			Id: r.PathValue("id"), Status: body.Status, TrackingCode: body.TrackingCode,
		})
		if err != nil {
			writeGRPCError(w, log, "Shop.UpdateOrderStatus", err)
			return
		}
		response.WriteJSON(w, http.StatusOK, toShopOrderDTO(o))
	}
}
