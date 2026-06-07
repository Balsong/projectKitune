package models

// BookingRequest — запрос на бронирование стола в ресторане.
type BookingRequest struct {
	Date   string `json:"date"`
	Time   string `json:"time"`
	Guests int    `json:"guests"`
	Phone  string `json:"phone"`
}
