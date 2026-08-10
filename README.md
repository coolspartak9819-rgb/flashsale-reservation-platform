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
- NATS JetStream publisher that marks Outbox rows only after broker acknowledgement;
- Prometheus-compatible metrics at `/metrics`;
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

When `NATS_URL` is configured, a background publisher sends Outbox events to the `flashsale.events` JetStream subject. Publishing is at-least-once: if the process stops after publishing but before marking the row, the event can be published again and consumers must use the event ID for deduplication. A 30-second publisher lease prevents two API replicas from claiming the same pending Outbox row at the same time.

Run the reservation load scenario after creating `flash-test` with 100 seats:

```bash
k6 run load/k6-reservations.js
```

The expected correctness signal is that responses are only `201 Created` or `409 Conflict`. A successful run must never oversell a seat.

For a dependency-free local benchmark, run the Go load client:

```bash
go run ./cmd/loadtest -requests 1000 -concurrency 100 -seats 100
```

The client reports throughput, p50, p95, p99 and HTTP status distribution. The benchmark output is environment-specific and should only be copied into the resume after a clean run.

One local in-memory baseline on a MacBook Pro:

| Metric | Result |
| --- | ---: |
| Requests | 1,000 |
| Concurrency | 100 |
| Successful reservations | 100 |
| Seat conflicts | 900 |
| Throughput | 33,434 req/s |
| p50 latency | 2.76 ms |
| p95 latency | 4.57 ms |
| p99 latency | 5.01 ms |

All 100 available seats were reserved exactly once. This is an in-memory correctness and client baseline, not a PostgreSQL capacity claim.

PostgreSQL-backed baseline from the same scenario:

| Metric | Result |
| --- | ---: |
| Requests | 1,000 |
| Concurrency | 100 |
| Successful reservations | 100 |
| Seat conflicts | 900 |
| Unexpected errors | 0 |
| Throughput | 3,268 req/s |
| p50 latency | 20.33 ms |
| p95 latency | 88.40 ms |
| p99 latency | 127.58 ms |

The API uses a maximum of 20 open PostgreSQL connections and queues excess work in the application pool instead of exhausting the database.

Confirm a payment:

```bash
curl -X POST http://localhost:8080/v1/reservations/<reservation_id>/confirm \\
  -H 'Content-Type: application/json' \\
  -d '{"payment_id":"payment-123"}'
```

Inspect metrics:

```bash
curl http://localhost:8080/metrics
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
