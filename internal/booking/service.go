// Package booking реализует Booking Service: хранение броней и эмиссию
// booking.requested через transactional outbox.
package booking

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	bookingv1 "tea-platform/internal/genpb/booking/v1"
	"tea-platform/internal/outbox"
	"tea-platform/pkg/events"
)

// Service реализует gRPC BookingService.
type Service struct {
	bookingv1.UnimplementedBookingServiceServer

	pool   *pgxpool.Pool
	outbox *outbox.Repo
	log    *slog.Logger
}

// NewService собирает сервис броней.
func NewService(pool *pgxpool.Pool, outboxRepo *outbox.Repo, log *slog.Logger) *Service {
	return &Service{pool: pool, outbox: outboxRepo, log: log}
}

type bookingRequestedPayload struct {
	BookingID string `json:"booking_id"`
	UserID    string `json:"user_id"`
	Customer  string `json:"customer"`
	Phone     string `json:"phone"`
	Guests    int32  `json:"guests"`
	Time      string `json:"time"`
}

// CreateBooking сохраняет бронь и эмитит booking.requested в одной транзакции.
func (s *Service) CreateBooking(ctx context.Context, req *bookingv1.CreateBookingRequest) (*bookingv1.Booking, error) {
	customer := strings.TrimSpace(req.GetCustomer())
	if customer == "" || strings.TrimSpace(req.GetTimeSlot()) == "" {
		return nil, status.Error(codes.InvalidArgument, "customer and time_slot are required")
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "begin: %v", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	const q = `
		INSERT INTO bookings (user_id, customer, phone, guests, time_slot, comment, status)
		VALUES ($1, $2, $3, $4, $5, $6, 'new')
		RETURNING id, created_at`
	var id string
	var createdAt time.Time
	if err := tx.QueryRow(ctx, q, req.GetUserId(), customer, req.GetPhone(), req.GetGuests(),
		req.GetTimeSlot(), req.GetComment()).Scan(&id, &createdAt); err != nil {
		return nil, status.Errorf(codes.Internal, "insert booking: %v", err)
	}

	env, err := events.New(events.EventBookingRequested, 1, id, bookingRequestedPayload{
		BookingID: id, UserID: req.GetUserId(), Customer: customer,
		Phone: req.GetPhone(), Guests: req.GetGuests(), Time: req.GetTimeSlot(),
	})
	if err != nil {
		return nil, status.Errorf(codes.Internal, "build event: %v", err)
	}
	if err := outbox.SaveTx(ctx, tx, outbox.SourceBooking, events.TopicBookings, env); err != nil {
		return nil, status.Errorf(codes.Internal, "outbox: %v", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, status.Errorf(codes.Internal, "commit: %v", err)
	}

	s.log.Info("бронь создана", "booking_id", id, "user_id", req.GetUserId(), "guests", req.GetGuests())
	return &bookingv1.Booking{
		Id: id, UserId: req.GetUserId(), Customer: customer, Phone: req.GetPhone(),
		Guests: req.GetGuests(), TimeSlot: req.GetTimeSlot(), Comment: req.GetComment(),
		Status: "new", CreatedAt: createdAt.UTC().Format(time.RFC3339),
	}, nil
}

// ListBookings возвращает брони пользователя (новые первыми).
func (s *Service) ListBookings(ctx context.Context, req *bookingv1.ListBookingsRequest) (*bookingv1.ListBookingsResponse, error) {
	if req.GetUserId() == "" {
		return nil, status.Error(codes.InvalidArgument, "user_id is required")
	}
	const q = `
		SELECT id, user_id, customer, phone, guests, time_slot, comment, status, created_at
		FROM bookings WHERE user_id = $1 ORDER BY created_at DESC LIMIT 50`
	rows, err := s.pool.Query(ctx, q, req.GetUserId())
	if err != nil {
		return nil, status.Errorf(codes.Internal, "list bookings: %v", err)
	}
	defer rows.Close()

	var out []*bookingv1.Booking
	for rows.Next() {
		var b bookingv1.Booking
		var createdAt time.Time
		if err := rows.Scan(&b.Id, &b.UserId, &b.Customer, &b.Phone, &b.Guests,
			&b.TimeSlot, &b.Comment, &b.Status, &createdAt); err != nil {
			return nil, status.Errorf(codes.Internal, "scan booking: %v", err)
		}
		b.CreatedAt = createdAt.UTC().Format(time.RFC3339)
		out = append(out, &b)
	}
	return &bookingv1.ListBookingsResponse{Bookings: out}, rows.Err()
}

// ListAllBookings возвращает все брони (админка), новые первыми.
func (s *Service) ListAllBookings(ctx context.Context, req *bookingv1.ListAllBookingsRequest) (*bookingv1.ListBookingsResponse, error) {
	limit := int(req.GetLimit())
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	const q = `
		SELECT id, user_id, customer, phone, guests, time_slot, comment, status, created_at
		FROM bookings ORDER BY created_at DESC LIMIT $1`
	rows, err := s.pool.Query(ctx, q, limit)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "list all bookings: %v", err)
	}
	defer rows.Close()

	var out []*bookingv1.Booking
	for rows.Next() {
		var b bookingv1.Booking
		var createdAt time.Time
		if err := rows.Scan(&b.Id, &b.UserId, &b.Customer, &b.Phone, &b.Guests,
			&b.TimeSlot, &b.Comment, &b.Status, &createdAt); err != nil {
			return nil, status.Errorf(codes.Internal, "scan booking: %v", err)
		}
		b.CreatedAt = createdAt.UTC().Format(time.RFC3339)
		out = append(out, &b)
	}
	return &bookingv1.ListBookingsResponse{Bookings: out}, rows.Err()
}

// validBookingStatus — допустимые статусы для админского перехода.
var validBookingStatus = map[string]bool{"new": true, "confirmed": true, "cancelled": true, "done": true}

// UpdateBookingStatus меняет статус брони (админка).
func (s *Service) UpdateBookingStatus(ctx context.Context, req *bookingv1.UpdateBookingStatusRequest) (*bookingv1.Booking, error) {
	if req.GetId() == "" || !validBookingStatus[req.GetStatus()] {
		return nil, status.Error(codes.InvalidArgument, "valid id and status are required")
	}
	const q = `
		UPDATE bookings SET status = $2 WHERE id = $1
		RETURNING id, user_id, customer, phone, guests, time_slot, comment, status, created_at`
	var b bookingv1.Booking
	var createdAt time.Time
	err := s.pool.QueryRow(ctx, q, req.GetId(), req.GetStatus()).Scan(
		&b.Id, &b.UserId, &b.Customer, &b.Phone, &b.Guests,
		&b.TimeSlot, &b.Comment, &b.Status, &createdAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, status.Errorf(codes.NotFound, "booking %s not found", req.GetId())
	}
	if err != nil {
		return nil, status.Errorf(codes.Internal, "update booking: %v", err)
	}
	b.CreatedAt = createdAt.UTC().Format(time.RFC3339)
	s.log.Info("статус брони изменён", "booking_id", b.Id, "status", b.Status)
	return &b, nil
}
