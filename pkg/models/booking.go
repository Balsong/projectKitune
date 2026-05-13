// pkg/models/menu.go
package models

type MenuItem struct {
	ID    int    `json:"id"`
	Name  string `json:"name"`
	Type  string `json:"type"` // tea, hot, dim_sum, dessert
	Price int    `json:"price"` // в копейках
}

// pkg/models/booking.go
package models

type BookingRequest struct {
	Date   string `json:"date"`
	Time   string `json:"time"`
	Guests int    `json:"guests"`
	Phone  string `json:"phone"`
}