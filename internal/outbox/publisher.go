package outbox

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"log/slog"
	"time"

	"github.com/coolspartak9819-rgb/flashsale-reservation-platform/internal/store"
	"github.com/nats-io/nats.go"
)

type Publisher struct {
	store *store.PostgresStore
	js    nats.JetStreamContext
	log   *slog.Logger
}

func NewPublisher(repository *store.PostgresStore, js nats.JetStreamContext, logger *slog.Logger) *Publisher {
	return &Publisher{store: repository, js: js, log: logger}
}

func (p *Publisher) Run(ctx context.Context) {
	for {
		if err := p.publishOne(ctx); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				select {
				case <-ctx.Done():
					return
				case <-time.After(300 * time.Millisecond):
					continue
				}
			}
			p.log.Error("outbox publish failed", "error", err)
			select {
			case <-ctx.Done():
				return
			case <-time.After(time.Second):
			}
		}
	}
}

func (p *Publisher) publishOne(ctx context.Context) error {
	event, err := p.store.ClaimOutbox(ctx)
	if err != nil {
		return err
	}
	body, err := json.Marshal(map[string]any{"id": event.ID, "type": event.EventType, "aggregate_id": event.AggregateID, "payload": json.RawMessage(event.Payload)})
	if err != nil {
		return err
	}
	if _, err = p.js.Publish("flashsale.events", body); err != nil {
		return err
	}
	return p.store.MarkOutboxPublished(ctx, event.ID)
}
