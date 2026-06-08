package events

// Топики Kafka платформы.
const (
	TopicBookings      = "bookings.events"
	TopicOrders        = "orders.events"
	TopicPayments      = "payments.events"
	TopicInventory     = "inventory.events"
	TopicNotifications = "notifications.commands"
)

// Типы событий.
const (
	// EventBookingRequested — пользователь запросил бронь стола.
	EventBookingRequested = "booking.requested"

	// EventOrderCreated — заказ создан (старт саги оформления).
	EventOrderCreated = "order.created"
)
