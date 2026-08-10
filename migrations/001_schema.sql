CREATE TABLE IF NOT EXISTS flash_events (
    event_id text PRIMARY KEY,
    name text NOT NULL,
    seat_count integer NOT NULL CHECK (seat_count > 0),
    created_at timestamptz NOT NULL
);

CREATE TABLE IF NOT EXISTS flash_reservations (
    reservation_id text PRIMARY KEY,
    event_id text NOT NULL REFERENCES flash_events(event_id),
    seat_id text NOT NULL,
    user_id text NOT NULL,
    idempotency_key text NOT NULL,
    status text NOT NULL CHECK (status IN ('active', 'expired', 'confirmed')),
    created_at timestamptz NOT NULL,
    expires_at timestamptz NOT NULL,
    UNIQUE (event_id, seat_id),
    UNIQUE (event_id, idempotency_key)
);

CREATE INDEX IF NOT EXISTS flash_reservations_expiry_idx ON flash_reservations (expires_at) WHERE status = 'active';

CREATE TABLE IF NOT EXISTS flash_outbox (
    id bigserial PRIMARY KEY,
    event_type text NOT NULL,
    aggregate_id text NOT NULL,
    payload jsonb NOT NULL,
    created_at timestamptz NOT NULL,
    published_at timestamptz
);

CREATE INDEX IF NOT EXISTS flash_outbox_pending_idx ON flash_outbox (created_at) WHERE published_at IS NULL;
