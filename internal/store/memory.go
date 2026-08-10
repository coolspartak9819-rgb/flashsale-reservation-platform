package store

import (
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/coolspartak9819-rgb/flashsale-reservation-platform/internal/domain"
)

var (
	ErrEventExists         = errors.New("event already exists")
	ErrEventNotFound       = errors.New("event not found")
	ErrSeatTaken           = errors.New("seat is already reserved")
	ErrInvalidSeat         = errors.New("seat is outside event capacity")
	ErrReservationNotFound = errors.New("reservation not found")
	ErrReservationExpired  = errors.New("reservation has expired")
	ErrReservationState    = errors.New("reservation cannot be confirmed in its current state")
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
	ConfirmReservation(string, string) (domain.Reservation, error)
	ExpireReservations(time.Time) (int, error)
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

func (s *Store) ConfirmReservation(id, paymentID string) (domain.Reservation, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	reservation, ok := s.reservations[id]
	if !ok {
		return domain.Reservation{}, ErrReservationNotFound
	}
	if reservation.Status == domain.ReservationConfirmed {
		return reservation, nil
	}
	if reservation.Status != domain.ReservationActive {
		return domain.Reservation{}, ErrReservationState
	}
	if !reservation.ExpiresAt.After(time.Now()) {
		reservation.Status = domain.ReservationExpired
		s.reservations[id] = reservation
		return domain.Reservation{}, ErrReservationExpired
	}
	reservation.Status = domain.ReservationConfirmed
	s.reservations[id] = reservation
	return reservation, nil
}

func (s *Store) ExpireReservations(now time.Time) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	count := 0
	for id, reservation := range s.reservations {
		if reservation.Status == domain.ReservationActive && !reservation.ExpiresAt.After(now) {
			reservation.Status = domain.ReservationExpired
			s.reservations[id] = reservation
			count++
		}
	}
	return count, nil
}
