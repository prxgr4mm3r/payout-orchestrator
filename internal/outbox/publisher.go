package outbox

import (
	"context"
	"errors"
	"log"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/prxgr4mm3r/payout-orchestrator/internal/db"
)

type Store interface {
	ClaimNextPendingOutboxEvent(ctx context.Context, reclaimBefore pgtype.Timestamptz) (db.OutboxEvent, error)
	MarkOutboxEventAsProcessed(ctx context.Context, id pgtype.UUID) (db.OutboxEvent, error)
	ReleaseOutboxEventClaim(ctx context.Context, id pgtype.UUID) (db.OutboxEvent, error)
}

type TxRunner interface {
	WithinTx(ctx context.Context, fn func(store Store) error) error
}

type EventPublisher interface {
	Publish(ctx context.Context, event PublishableEvent) error
}

type EventPublisherFunc func(ctx context.Context, event PublishableEvent) error

func (f EventPublisherFunc) Publish(ctx context.Context, event PublishableEvent) error {
	return f(ctx, event)
}

type PublishedEventHandler interface {
	HandlePublishedEvent(ctx context.Context, event PublishableEvent) error
}

type PublishableEvent struct {
	ID        string
	EventType string
	EntityID  string
	Payload   []byte
}

type Config struct {
	PollInterval time.Duration
	ClaimTimeout time.Duration
}

type Publisher struct {
	txRunner  TxRunner
	publisher EventPublisher
	logger    *log.Logger
	config    Config
}

func NewPublisher(txRunner TxRunner, publisher EventPublisher, logger *log.Logger, config Config) *Publisher {
	if logger == nil {
		logger = log.Default()
	}
	if config.PollInterval <= 0 {
		config.PollInterval = time.Second
	}
	if config.ClaimTimeout <= 0 {
		config.ClaimTimeout = 30 * time.Second
	}

	return &Publisher{
		txRunner:  txRunner,
		publisher: publisher,
		logger:    logger,
		config:    config,
	}
}

func NewInlinePublisher(handler PublishedEventHandler) EventPublisher {
	return EventPublisherFunc(func(ctx context.Context, event PublishableEvent) error {
		if handler == nil {
			return errors.New("outbox inline publisher is not configured")
		}

		return handler.HandlePublishedEvent(ctx, event)
	})
}

func (p *Publisher) Run(ctx context.Context) error {
	if p == nil || p.txRunner == nil || p.publisher == nil {
		return errors.New("outbox publisher is not configured")
	}

	ticker := time.NewTicker(p.config.PollInterval)
	defer ticker.Stop()

	for {
		published, err := p.RunOnce(ctx)
		if err != nil {
			return err
		}
		if published {
			continue
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}

func (p *Publisher) RunOnce(ctx context.Context) (bool, error) {
	if p == nil || p.txRunner == nil || p.publisher == nil {
		return false, errors.New("outbox publisher is not configured")
	}

	reclaimBefore := pgtype.Timestamptz{
		Time:  time.Now().Add(-p.config.ClaimTimeout),
		Valid: true,
	}

	event, claimed, err := p.claimNextPendingEvent(ctx, reclaimBefore)
	if err != nil {
		return false, err
	}
	if !claimed {
		return false, nil
	}

	publishable := PublishableEvent{
		ID:        event.ID.String(),
		EventType: event.EventType,
		EntityID:  event.EntityID.String(),
		Payload:   event.Payload,
	}

	if err := p.publisher.Publish(ctx, publishable); err != nil {
		if releaseErr := p.releaseClaim(ctx, event.ID); releaseErr != nil {
			return false, releaseErr
		}

		return false, err
	}

	if err := p.markProcessed(ctx, event.ID); err != nil {
		return false, err
	}

	p.logger.Printf("published outbox event id=%s type=%s", publishable.ID, publishable.EventType)
	return true, nil
}

func (p *Publisher) claimNextPendingEvent(ctx context.Context, reclaimBefore pgtype.Timestamptz) (db.OutboxEvent, bool, error) {
	var event db.OutboxEvent

	err := p.txRunner.WithinTx(ctx, func(store Store) error {
		claimed, err := store.ClaimNextPendingOutboxEvent(ctx, reclaimBefore)
		if err != nil {
			return err
		}

		event = claimed
		return nil
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return db.OutboxEvent{}, false, nil
	}
	if err != nil {
		return db.OutboxEvent{}, false, err
	}

	return event, true, nil
}

func (p *Publisher) markProcessed(ctx context.Context, id pgtype.UUID) error {
	return p.txRunner.WithinTx(ctx, func(store Store) error {
		_, err := store.MarkOutboxEventAsProcessed(ctx, id)
		return err
	})
}

func (p *Publisher) releaseClaim(ctx context.Context, id pgtype.UUID) error {
	return p.txRunner.WithinTx(ctx, func(store Store) error {
		_, err := store.ReleaseOutboxEventClaim(ctx, id)
		return err
	})
}
