package domain

import "time"

type Event struct {
	ID        string    `json:"event_id"`
	Name      string    `json:"name"`
	SeatCount int       `json:"seat_count"`
	CreatedAt time.Time `json:"created_at"`
}

type Reservation struct {
	ID            string    `json:"reservation_id"`
	EventID       string    `json:"event_id"`
	SeatID        string    `json:"seat_id"`
	UserID        string    `json:"user_id"`
	Status        string    `json:"status"`
	CreatedAt     time.Time `json:"created_at"`
	ExpiresAt     time.Time `json:"expires_at"`
	IdempotentHit bool      `json:"idempotent_hit,omitempty"`
}

const (
	ReservationActive    = "active"
	ReservationExpired   = "expired"
	ReservationConfirmed = "confirmed"
)
