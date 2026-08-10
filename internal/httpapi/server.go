package httpapi

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/coolspartak9819-rgb/flashsale-reservation-platform/internal/domain"
	"github.com/coolspartak9819-rgb/flashsale-reservation-platform/internal/metrics"
	"github.com/coolspartak9819-rgb/flashsale-reservation-platform/internal/store"
)

type Server struct {
	store   store.ReservationStore
	metrics *metrics.Metrics
}

func New(repository store.ReservationStore, telemetry ...*metrics.Metrics) http.Handler {
	serviceMetrics := &metrics.Metrics{}
	if len(telemetry) > 0 && telemetry[0] != nil {
		serviceMetrics = telemetry[0]
	}
	s := &Server{store: repository, metrics: serviceMetrics}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", s.health)
	mux.HandleFunc("GET /metrics", s.metricsHandler)
	mux.HandleFunc("POST /v1/events", s.createEvent)
	mux.HandleFunc("POST /v1/events/", s.reserve)
	mux.HandleFunc("POST /v1/reservations/", s.confirm)
	return mux
}

func (s *Server) confirm(w http.ResponseWriter, r *http.Request) {
	s.metrics.Requests.Add(1)
	parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
	if len(parts) != 4 || parts[0] != "v1" || parts[1] != "reservations" || parts[3] != "confirm" {
		s.metrics.Failures.Add(1)
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "route not found"})
		return
	}
	var request struct {
		PaymentID string `json:"payment_id"`
	}
	if !decode(r, &request) || request.PaymentID == "" {
		s.metrics.Failures.Add(1)
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "payment_id is required"})
		return
	}
	reservation, err := s.store.ConfirmReservation(parts[2], request.PaymentID)
	if err != nil {
		s.metrics.Failures.Add(1)
		status := http.StatusConflict
		if errors.Is(err, store.ErrReservationNotFound) {
			status = http.StatusNotFound
		}
		writeJSON(w, status, map[string]string{"error": err.Error()})
		return
	}
	s.metrics.Confirmed.Add(1)
	writeJSON(w, http.StatusOK, reservation)
}

func (s *Server) health(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) metricsHandler(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/plain; version=0.0.4")
	_, _ = w.Write([]byte(s.metrics.Render()))
}

func (s *Server) createEvent(w http.ResponseWriter, r *http.Request) {
	s.metrics.Requests.Add(1)
	var event domain.Event
	if !decode(r, &event) || event.ID == "" || event.Name == "" || event.SeatCount < 1 {
		s.metrics.Failures.Add(1)
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "event_id, name and positive seat_count are required"})
		return
	}
	event.CreatedAt = time.Now().UTC()
	if err := s.store.CreateEvent(event); err != nil {
		s.metrics.Failures.Add(1)
		status := http.StatusConflict
		writeJSON(w, status, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusCreated, event)
}

func (s *Server) reserve(w http.ResponseWriter, r *http.Request) {
	s.metrics.Requests.Add(1)
	parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
	if len(parts) != 4 || parts[0] != "v1" || parts[1] != "events" || parts[3] != "reservations" {
		s.metrics.Failures.Add(1)
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "route not found"})
		return
	}
	var request struct {
		UserID string `json:"user_id"`
		SeatID string `json:"seat_id"`
	}
	if !decode(r, &request) || request.UserID == "" || request.SeatID == "" {
		s.metrics.Failures.Add(1)
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "user_id and seat_id are required"})
		return
	}
	key := r.Header.Get("Idempotency-Key")
	if key == "" {
		s.metrics.Failures.Add(1)
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Idempotency-Key header is required"})
		return
	}
	reservation, err := s.store.Reserve(parts[2], request.UserID, request.SeatID, key, 10*time.Minute)
	if err != nil {
		if errors.Is(err, store.ErrSeatTaken) {
			s.metrics.Conflicts.Add(1)
		} else {
			s.metrics.Failures.Add(1)
		}
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
	s.metrics.Created.Add(1)
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
