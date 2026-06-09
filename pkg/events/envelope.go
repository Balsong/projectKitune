// Package events описывает единый конверт событий платформы и общие
// константы топиков/типов, используемые продюсерами и консьюмерами Kafka.
package events

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

// Envelope — единый конверт сообщения во всех топиках. Полезная нагрузка
// (Payload) специфична для типа события и хранится сырым JSON.
type Envelope struct {
	EventID       string          `json:"event_id"`       // уникальный id для дедупликации
	EventType     string          `json:"event_type"`     // напр. "booking.requested"
	EventVersion  int             `json:"event_version"`  // версия схемы payload
	AggregateID   string          `json:"aggregate_id"`   // id агрегата (order_id, booking_id)
	OccurredAt    time.Time       `json:"occurred_at"`    // момент возникновения (UTC)
	CorrelationID string          `json:"correlation_id"` // сквозной id через всю сагу
	CausationID   string          `json:"causation_id,omitempty"`
	Payload       json.RawMessage `json:"payload"`
}

// New собирает конверт нового события, сериализуя payload в JSON.
// Генерирует event_id и correlation_id; aggregateID задаётся вызывающим.
func New(eventType string, version int, aggregateID string, payload any) (Envelope, error) {
	raw, err := json.Marshal(payload)
	if err != nil {
		return Envelope{}, err
	}
	return Envelope{
		EventID:       NewID(),
		EventType:     eventType,
		EventVersion:  version,
		AggregateID:   aggregateID,
		OccurredAt:    time.Now().UTC(),
		CorrelationID: NewID(),
		Payload:       raw,
	}, nil
}

// NewFollow собирает событие как следствие parent: переносит correlation_id
// (сквозной id саги) и проставляет causation_id = parent.EventID.
func NewFollow(eventType string, version int, aggregateID string, payload any, parent Envelope) (Envelope, error) {
	env, err := New(eventType, version, aggregateID, payload)
	if err != nil {
		return Envelope{}, err
	}
	env.CorrelationID = parent.CorrelationID
	env.CausationID = parent.EventID
	return env, nil
}

// NewWithCorrelation собирает событие с заданным correlation_id (когда нет
// родительского конверта — например, фоновый воркер продолжает сагу).
func NewWithCorrelation(eventType string, version int, aggregateID, correlationID string, payload any) (Envelope, error) {
	env, err := New(eventType, version, aggregateID, payload)
	if err != nil {
		return Envelope{}, err
	}
	if correlationID != "" {
		env.CorrelationID = correlationID
	}
	return env, nil
}

// UnmarshalPayload разбирает полезную нагрузку конверта в target.
func (e Envelope) UnmarshalPayload(target any) error {
	return json.Unmarshal(e.Payload, target)
}

// NewID возвращает новый UUID v4 в строковом виде.
func NewID() string {
	return uuid.NewString()
}
