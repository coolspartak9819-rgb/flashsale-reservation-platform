package store

import (
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/coolspartak9819-rgb/flashsale-reservation-platform/internal/domain"
)

var (
	ErrEventExists   = errors.New("event already exists")
	ErrEventNotFound = errors.New("event not found")
	ErrSeatTaken     = errors.New("seat is already reserved")
	ErrInvalidSeat   = errors.New("seat is outside event capacity")
)

type Store struct {
	mu            sync.Mutex
	events        map[string]domain.Event
	reservations  map[string]domain.Reservation
	byIdempotency map[string]string
	bySeat        map[string]string
}

type ReservationStore interface {
	CreateEvent(domain.Event) error
	Reserve(string, string, string, string, time.Duration) (domain.Reservation, error)
}

func New() *Store {
	return &Store{
		events:        make(map[string]domain.Event),
		reservations:  make(map[string]domain.Reservation),
		byIdempotency: make(map[string]string),
		bySeat:        make(map[string]string),
	}
}

func (s *Store) CreateEvent(event domain.Event) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.events[event.ID]; exists {
		return ErrEventExists
	}
	s.events[event.ID] = event
	return nil
}

func (s *Store) Reserve(eventID, userID, seatID, idempotencyKey string, ttl time.Duration) (domain.Reservation, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.events[eventID]; !ok {
		return domain.Reservation{}, ErrEventNotFound
	}
	idempotencyID := eventID + ":" + idempotencyKey
	if existingID, ok := s.byIdempotency[idempotencyID]; ok {
		reservation := s.reservations[existingID]
		reservation.IdempotentHit = true
		return reservation, nil
	}
	event := s.events[eventID]
	var seatNumber int
	if _, err := fmt.Sscanf(seatID, "seat-%d", &seatNumber); err != nil || seatNumber < 1 || seatNumber > event.SeatCount {
		return domain.Reservation{}, ErrInvalidSeat
	}
	seatKey := eventID + ":" + seatID
	if existingID, ok := s.bySeat[seatKey]; ok {
		reservation := s.reservations[existingID]
		if reservation.ExpiresAt.After(time.Now()) && reservation.Status == domain.ReservationActive {
			return domain.Reservation{}, ErrSeatTaken
		}
		delete(s.reservations, existingID)
	}
	now := time.Now().UTC()
	reservation := domain.Reservation{
		ID:      "res_" + eventID + "_" + seatID + "_" + userID,
		EventID: eventID, SeatID: seatID, UserID: userID,
		Status: domain.ReservationActive, CreatedAt: now, ExpiresAt: now.Add(ttl),
	}
	s.reservations[reservation.ID] = reservation
	s.byIdempotency[idempotencyID] = reservation.ID
	s.bySeat[seatKey] = reservation.ID
	return reservation, nil
}
