package httpapi

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/coolspartak9819-rgb/flashsale-reservation-platform/internal/domain"
	"github.com/coolspartak9819-rgb/flashsale-reservation-platform/internal/store"
)

type Server struct {
	store *store.Store
}

func New(store *store.Store) http.Handler {
	s := &Server{store: store}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", s.health)
	mux.HandleFunc("POST /v1/events", s.createEvent)
	mux.HandleFunc("POST /v1/events/", s.reserve)
	return mux
}

func (s *Server) health(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) createEvent(w http.ResponseWriter, r *http.Request) {
	var event domain.Event
	if !decode(r, &event) || event.ID == "" || event.Name == "" || event.SeatCount < 1 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "event_id, name and positive seat_count are required"})
		return
	}
	event.CreatedAt = time.Now().UTC()
	if err := s.store.CreateEvent(event); err != nil {
		status := http.StatusConflict
		writeJSON(w, status, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusCreated, event)
}

func (s *Server) reserve(w http.ResponseWriter, r *http.Request) {
	parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
	if len(parts) != 4 || parts[0] != "v1" || parts[1] != "events" || parts[3] != "reservations" {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "route not found"})
		return
	}
	var request struct {
		UserID string `json:"user_id"`
		SeatID string `json:"seat_id"`
	}
	if !decode(r, &request) || request.UserID == "" || request.SeatID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "user_id and seat_id are required"})
		return
	}
	key := r.Header.Get("Idempotency-Key")
	if key == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Idempotency-Key header is required"})
		return
	}
	reservation, err := s.store.Reserve(parts[2], request.UserID, request.SeatID, key, 10*time.Minute)
	if err != nil {
		status := http.StatusBadRequest
		if errors.Is(err, store.ErrEventNotFound) {
			status = http.StatusNotFound
		}
		if errors.Is(err, store.ErrSeatTaken) {
			status = http.StatusConflict
		}
		writeJSON(w, status, map[string]string{"error": err.Error()})
		return
	}
	status := http.StatusCreated
	if reservation.IdempotentHit {
		status = http.StatusOK
	}
	writeJSON(w, status, reservation)
}

func decode(r *http.Request, target any) bool { return json.NewDecoder(r.Body).Decode(target) == nil }

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
