package outbox

import "time"

type OutboxEvent struct {
	ID            int64      `db:"id" json:"id"`
	EventID       string     `db:"event_id"  json:"event_id"`
	EventType     string     `db:"event_type"  json:"event_type"`
	Payload       []byte     `db:"payload"     json:"payload"`
	AggregateType string     `db:"aggregate_type"  json:"aggregate_type"`
	AggregateID   string     `db:"aggregate_id"   json:"aggregate_id"`
	Status        string     `db:"status"         json:"status"`
	LockedUntil   *time.Time `db:"locked_until"    json:"locked_until"`

	CreatedAt   time.Time  `db:"created_at"  json:"created_at"`
	ProcessedAt *time.Time `db:"processed_at"  json:"processed_at"`
}
