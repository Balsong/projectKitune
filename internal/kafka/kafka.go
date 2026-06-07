package kafka

import (
	"encoding/json"
	"log"
)

// Producer — продюсер сообщений в Kafka.
// На Этапе 0 это заглушка; реальную отправку через segmentio/kafka-go
// добавим вместе с transactional outbox.
type Producer struct{}

// NewProducer создаёт продюсера для указанных брокеров.
func NewProducer(brokers string) (*Producer, error) {
	log.Printf("🕊️ [Kafka] Подготовка продюсера для брокеров: %s", brokers)
	return &Producer{}, nil
}

// Send отправляет сырой payload в указанный топик.
func (p *Producer) Send(topic string, payload []byte) error {
	// Здесь будет реальная отправка в Kafka
	log.Printf("📨 [Kafka] Отправка в топик %s: %s", topic, string(payload))
	return nil
}

// SendBooking сериализует бронь и публикует её в топик бронирований.
func (p *Producer) SendBooking(booking any) error {
	payload, err := json.Marshal(booking)
	if err != nil {
		return err
	}
	return p.Send("bookings.events", payload)
}

// Close освобождает ресурсы продюсера.
func (p *Producer) Close() error {
	log.Printf("🕊️ [Kafka] Закрытие продюсера")
	return nil
}
