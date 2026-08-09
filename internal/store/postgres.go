package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	"github.com/coolspartak9819-rgb/flashsale-reservation-platform/internal/domain"
)

type PostgresStore struct{ db *sql.DB }

func NewPostgresStore(db *sql.DB) *PostgresStore { return &PostgresStore{db: db} }

func (s *PostgresStore) CreateEvent(event domain.Event) error {
	_, err := s.db.Exec(`INSERT INTO flash_events (event_id, name, seat_count, created_at) VALUES ($1, $2, $3, $4)`, event.ID, event.Name, event.SeatCount, event.CreatedAt)
	if err != nil {
		if isUniqueViolation(err) {
			return ErrEventExists
		}
		return err
	}
	return nil
}

func (s *PostgresStore) Reserve(eventID, userID, seatID, idempotencyKey string, ttl time.Duration) (domain.Reservation, error) {
	ctx := context.Background()
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	if err != nil {
		return domain.Reservation{}, err
	}
	defer tx.Rollback()

	var eventExists bool
	if err := tx.QueryRow(`SELECT EXISTS (SELECT 1 FROM flash_events WHERE event_id = $1)`, eventID).Scan(&eventExists); err != nil {
		return domain.Reservation{}, err
	}
	if !eventExists {
		return domain.Reservation{}, ErrEventNotFound
	}

	var existing domain.Reservation
	err = tx.QueryRow(`SELECT reservation_id, event_id, seat_id, user_id, status, created_at, expires_at FROM flash_reservations WHERE event_id = $1 AND idempotency_key = $2`, eventID, idempotencyKey).Scan(&existing.ID, &existing.EventID, &existing.SeatID, &existing.UserID, &existing.Status, &existing.CreatedAt, &existing.ExpiresAt)
	if err == nil {
		existing.IdempotentHit = true
		return existing, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return domain.Reservation{}, err
	}

	var existingID, existingUser, existingStatus string
	var existingExpires time.Time
	err = tx.QueryRow(`SELECT reservation_id, user_id, status, expires_at FROM flash_reservations WHERE event_id = $1 AND seat_id = $2 FOR UPDATE`, eventID, seatID).Scan(&existingID, &existingUser, &existingStatus, &existingExpires)
	if err == nil && existingStatus == domain.ReservationActive && existingExpires.After(time.Now().UTC()) {
		return domain.Reservation{}, ErrSeatTaken
	}
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return domain.Reservation{}, err
	}

	now := time.Now().UTC()
	reservation := domain.Reservation{ID: "res_" + eventID + "_" + seatID + "_" + userID, EventID: eventID, SeatID: seatID, UserID: userID, Status: domain.ReservationActive, CreatedAt: now, ExpiresAt: now.Add(ttl)}
	if errors.Is(err, sql.ErrNoRows) {
		_, err = tx.Exec(`INSERT INTO flash_reservations (reservation_id, event_id, seat_id, user_id, idempotency_key, status, created_at, expires_at) VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`, reservation.ID, eventID, seatID, userID, idempotencyKey, reservation.Status, now, reservation.ExpiresAt)
	} else {
		_, err = tx.Exec(`UPDATE flash_reservations SET reservation_id = $1, user_id = $2, idempotency_key = $3, status = $4, created_at = $5, expires_at = $6 WHERE event_id = $7 AND seat_id = $8`, reservation.ID, userID, idempotencyKey, reservation.Status, now, reservation.ExpiresAt, eventID, seatID)
	}
	if err != nil {
		if isUniqueViolation(err) {
			return domain.Reservation{}, ErrSeatTaken
		}
		return domain.Reservation{}, err
	}
	if err := addOutbox(tx, "reservation.created", reservation.ID, reservation); err != nil {
		return domain.Reservation{}, err
	}
	if err := tx.Commit(); err != nil {
		return domain.Reservation{}, err
	}
	return reservation, nil
}

func (s *PostgresStore) ConfirmReservation(id, paymentID string) (domain.Reservation, error) {
	tx, err := s.db.Begin()
	if err != nil {
		return domain.Reservation{}, err
	}
	defer tx.Rollback()
	var reservation domain.Reservation
	err = tx.QueryRow(`SELECT reservation_id, event_id, seat_id, user_id, status, created_at, expires_at FROM flash_reservations WHERE reservation_id = $1 FOR UPDATE`, id).Scan(&reservation.ID, &reservation.EventID, &reservation.SeatID, &reservation.UserID, &reservation.Status, &reservation.CreatedAt, &reservation.ExpiresAt)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.Reservation{}, ErrReservationNotFound
	}
	if err != nil {
		return domain.Reservation{}, err
	}
	if reservation.Status == domain.ReservationConfirmed {
		return reservation, nil
	}
	if reservation.Status != domain.ReservationActive {
		return domain.Reservation{}, ErrReservationState
	}
	if !reservation.ExpiresAt.After(time.Now().UTC()) {
		_, err = tx.Exec(`UPDATE flash_reservations SET status = $1 WHERE reservation_id = $2`, domain.ReservationExpired, id)
		if err != nil {
			return domain.Reservation{}, err
		}
		return domain.Reservation{}, ErrReservationExpired
	}
	_, err = tx.Exec(`UPDATE flash_reservations SET status = $1 WHERE reservation_id = $2`, domain.ReservationConfirmed, id)
	if err != nil {
		return domain.Reservation{}, err
	}
	if err = addOutbox(tx, "reservation.confirmed", id, map[string]string{"reservation_id": id, "payment_id": paymentID}); err != nil {
		return domain.Reservation{}, err
	}
	if err = tx.Commit(); err != nil {
		return domain.Reservation{}, err
	}
	reservation.Status = domain.ReservationConfirmed
	return reservation, nil
}

func (s *PostgresStore) ExpireReservations(now time.Time) (int, error) {
	tx, err := s.db.Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	rows, err := tx.Query(`SELECT reservation_id FROM flash_reservations WHERE status = $1 AND expires_at <= $2 FOR UPDATE SKIP LOCKED`, domain.ReservationActive, now)
	if err != nil {
		return 0, err
	}
	count := 0
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return count, err
		}
		if _, err := tx.Exec(`UPDATE flash_reservations SET status = $1 WHERE reservation_id = $2`, domain.ReservationExpired, id); err != nil {
			return count, err
		}
		if err := addOutbox(tx, "reservation.expired", id, map[string]string{"reservation_id": id}); err != nil {
			return count, err
		}
		count++
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return count, err
	}
	rows.Close()
	if err := tx.Commit(); err != nil {
		return count, err
	}
	return count, nil
}

func addOutbox(tx *sql.Tx, eventType, aggregateID string, payload any) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	_, err = tx.Exec(`INSERT INTO flash_outbox (event_type, aggregate_id, payload, created_at) VALUES ($1, $2, $3, now())`, eventType, aggregateID, body)
	return err
}

func isUniqueViolation(err error) bool {
	var pqErr interface{ SQLState() string }
	return errors.As(err, &pqErr) && pqErr.SQLState() == "23505"
}
