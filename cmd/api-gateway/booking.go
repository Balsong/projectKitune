package main

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	accountv1 "tea-platform/internal/genpb/account/v1"
	bookingv1 "tea-platform/internal/genpb/booking/v1"
	"tea-platform/pkg/response"
)

type bookingDTO struct {
	ID        string `json:"id"`
	Customer  string `json:"customer"`
	Phone     string `json:"phone"`
	Guests    int32  `json:"guests"`
	TimeSlot  string `json:"time_slot"`
	Comment   string `json:"comment"`
	Status    string `json:"status"`
	CreatedAt string `json:"created_at"`
}

func toBookingDTO(b *bookingv1.Booking) bookingDTO {
	return bookingDTO{
		ID: b.GetId(), Customer: b.GetCustomer(), Phone: b.GetPhone(), Guests: b.GetGuests(),
		TimeSlot: b.GetTimeSlot(), Comment: b.GetComment(), Status: b.GetStatus(), CreatedAt: b.GetCreatedAt(),
	}
}

// handleCreateBooking принимает бронь и сохраняет её через Booking Service.
// Если запрос авторизован — бронь привязывается к пользователю.
func handleCreateBooking(log *slog.Logger, account accountv1.AccountServiceClient, client bookingv1.BookingServiceClient) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Customer string `json:"customer"`
			Phone    string `json:"phone"`
			Guests   int32  `json:"guests"`
			Time     string `json:"time"`
			Comment  string `json:"comment"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			response.Error(w, http.StatusBadRequest, "Invalid request body")
			return
		}

		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()

		b, err := client.CreateBooking(ctx, &bookingv1.CreateBookingRequest{
			UserId:   sessionUserID(ctx, account, r),
			Customer: body.Customer, Phone: body.Phone, Guests: body.Guests,
			TimeSlot: body.Time, Comment: body.Comment,
		})
		if err != nil {
			writeGRPCError(w, log, "CreateBooking", err)
			return
		}
		response.WriteJSON(w, http.StatusAccepted, map[string]string{
			"status":     "booking_pending",
			"booking_id": b.GetId(),
			"message":    "Your booking is being processed",
		})
	}
}

// handleMyBookings возвращает брони текущего пользователя.
func handleMyBookings(log *slog.Logger, account accountv1.AccountServiceClient, client bookingv1.BookingServiceClient) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()

		uid := sessionUserID(ctx, account, r)
		if uid == "" {
			response.Error(w, http.StatusUnauthorized, "Требуется вход")
			return
		}
		resp, err := client.ListBookings(ctx, &bookingv1.ListBookingsRequest{UserId: uid})
		if err != nil {
			writeGRPCError(w, log, "ListBookings", err)
			return
		}
		out := make([]bookingDTO, 0, len(resp.GetBookings()))
		for _, b := range resp.GetBookings() {
			out = append(out, toBookingDTO(b))
		}
		response.WriteJSON(w, http.StatusOK, out)
	}
}
