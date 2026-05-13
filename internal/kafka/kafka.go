package kafka

import "log"

type Producer struct{}

func NewProducer(brokers string) (*Producer, error) {
	log.Printf("🕊️ [Kafka] Подготовка продюсера для брокеров: %s", brokers)
	return &Producer{}, nil
}

func (p *Producer) Send(topic string, payload []byte) error {
	// Здесь будет реальная отправка в Kafka
	log.Printf("📨 [Kafka] Отправка в топик %s: %s", topic, string(payload))
	return nil
}
