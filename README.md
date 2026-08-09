# Flash Sale Reservation Platform

Go service that protects a limited inventory during a traffic spike. The first slice focuses on the hardest correctness rule: one seat must never be reserved twice, even when many requests arrive at the same time.

## First slice

- event creation with a fixed seat capacity;
- seat reservation with a ten-minute TTL;
- idempotency through the `Idempotency-Key` header;
- PostgreSQL persistence with unique constraints for event seats and idempotency keys;
- transactional seat locking with `SELECT FOR UPDATE`;
- payment confirmation with `active`, `confirmed` and `expired` states;
- transactional Outbox records for reservation creation, confirmation and expiration;
- background expiration loop for abandoned reservations;
- concurrent reservation test with 100 goroutines;
- clear conflict responses for an already reserved seat.

## Run

```bash
go test -race ./...
go run ./cmd/api
```

Run the PostgreSQL-backed API with Docker Compose:

```bash
docker compose up --build
```

Run the reservation load scenario after creating `flash-test` with 100 seats:

```bash
k6 run load/k6-reservations.js
```

The expected correctness signal is that responses are only `201 Created` or `409 Conflict`. A successful run must never oversell a seat.

Confirm a payment:

```bash
curl -X POST http://localhost:8080/v1/reservations/<reservation_id>/confirm \\
  -H 'Content-Type: application/json' \\
  -d '{"payment_id":"payment-123"}'
```

Create an event:

```bash
curl -X POST http://localhost:8080/v1/events \
  -H 'Content-Type: application/json' \
  -d '{"event_id":"concert-1","name":"Summer Concert","seat_count":100}'
```

Reserve a seat:

```bash
curl -X POST http://localhost:8080/v1/events/concert-1/reservations \
  -H 'Content-Type: application/json' \
  -H 'Idempotency-Key: payment-attempt-1' \
  -d '{"user_id":"user-1","seat_id":"seat-42"}'
```

The next iterations will add an outbox and payment timeout flow, then measure oversell protection under load.
