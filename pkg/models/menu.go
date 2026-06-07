package models

// MenuItem — позиция меню/каталога.
// Type различает домены: tea (магазин), hot/dim_sum/dessert (ресторан).
type MenuItem struct {
	ID    int    `json:"id"`
	Name  string `json:"name"`
	Type  string `json:"type"`  // tea, hot, dim_sum, dessert
	Price int    `json:"price"` // в копейках
}
