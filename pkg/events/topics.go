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

	// EventStockReserved — остатки успешно зарезервированы под заказ.
	EventStockReserved = "stock.reserved"
	// EventReservationFailed — не удалось зарезервировать (нет в наличии/стоп-лист).
	EventReservationFailed = "reservation.failed"

	// EventPaymentSucceeded — оплата заказа прошла.
	EventPaymentSucceeded = "payment.succeeded"
	// EventPaymentFailed — оплата заказа не прошла.
	EventPaymentFailed = "payment.failed"

	// EventOrderConfirmed — заказ подтверждён (оплачен, резерв коммитится).
	EventOrderConfirmed = "order.confirmed"
	// EventOrderCancelled — заказ отменён (запуск компенсаций: release резерва).
	EventOrderCancelled = "order.cancelled"
)
