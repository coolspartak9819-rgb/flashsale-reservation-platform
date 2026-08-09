# Flash Sale Reservation Platform

Go service that protects a limited inventory during a traffic spike. The first slice focuses on the hardest correctness rule: one seat must never be reserved twice, even when many requests arrive at the same time.

## First slice

- event creation with a fixed seat capacity;
- seat reservation with a ten-minute TTL;
- idempotency through the `Idempotency-Key` header;
- concurrent reservation test with 100 goroutines;
- clear conflict responses for an already reserved seat.

## Run

```bash
go test -race ./...
go run ./cmd/api
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

The next iterations will replace the memory store with PostgreSQL transactions, add an outbox and payment timeout flow, then measure oversell protection under load.
