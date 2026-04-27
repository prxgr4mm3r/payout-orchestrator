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

type EventStore interface {
	ClaimNextPendingOutboxEvent(ctx context.Context, reclaimBefore pgtype.Timestamptz) (db.OutboxEvent, error)
	ClaimNextPendingOutboxEventByTypes(ctx context.Context, arg db.ClaimNextPendingOutboxEventByTypesParams) (db.OutboxEvent, error)
	MarkOutboxEventAsProcessed(ctx context.Context, id pgtype.UUID) (db.OutboxEvent, error)
	ReleaseOutboxEventClaim(ctx context.Context, id pgtype.UUID) (db.OutboxEvent, error)
}

type TransactionRunner interface {
	WithinTx(ctx context.Context, fn func(store EventStore) error) error
}

type EventDispatcher interface {
	Dispatch(ctx context.Context, event Event) error
}

type EventHandler interface {
	HandleEvent(ctx context.Context, event Event) error
}

type Event struct {
	ID        string
	EventType string
	EntityID  string
	Payload   []byte
}

type Config struct {
	PollInterval time.Duration
	ClaimTimeout time.Duration
	EventTypes   []string
}

type Relay struct {
	txRunner   TransactionRunner
	dispatcher EventDispatcher
	logger     *log.Logger
	config     Config
}

func NewRelay(txRunner TransactionRunner, dispatcher EventDispatcher, logger *log.Logger, config Config) *Relay {
	if logger == nil {
		logger = log.Default()
	}
	if config.PollInterval <= 0 {
		config.PollInterval = time.Second
	}
	if config.ClaimTimeout <= 0 {
		config.ClaimTimeout = 30 * time.Second
	}

	return &Relay{
		txRunner:   txRunner,
		dispatcher: dispatcher,
		logger:     logger,
		config:     config,
	}
}

type inlineDispatcher struct {
	handler EventHandler
}

func NewInlineDispatcher(handler EventHandler) EventDispatcher {
	return inlineDispatcher{handler: handler}
}

func (d inlineDispatcher) Dispatch(ctx context.Context, event Event) error {
	if d.handler == nil {
		return errors.New("outbox inline dispatcher is not configured")
	}

	return d.handler.HandleEvent(ctx, event)
}

func (r *Relay) Run(ctx context.Context) error {
	if r == nil || r.txRunner == nil || r.dispatcher == nil {
		return errors.New("outbox relay is not configured")
	}

	ticker := time.NewTicker(r.config.PollInterval)
	defer ticker.Stop()

	for {
		dispatched, err := r.RunOnce(ctx)
		if err != nil {
			return err
		}
		if dispatched {
			continue
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}

func (r *Relay) RunOnce(ctx context.Context) (bool, error) {
	if r == nil || r.txRunner == nil || r.dispatcher == nil {
		return false, errors.New("outbox relay is not configured")
	}

	reclaimBefore := pgtype.Timestamptz{
		Time:  time.Now().Add(-r.config.ClaimTimeout),
		Valid: true,
	}

	event, claimed, err := r.claimNextPendingEvent(ctx, reclaimBefore)
	if err != nil {
		return false, err
	}
	if !claimed {
		return false, nil
	}

	dispatchEvent := Event{
		ID:        event.ID.String(),
		EventType: event.EventType,
		EntityID:  event.EntityID.String(),
		Payload:   event.Payload,
	}

	if err := r.dispatcher.Dispatch(ctx, dispatchEvent); err != nil {
		if releaseErr := r.releaseClaim(ctx, event.ID); releaseErr != nil {
			return false, releaseErr
		}

		return false, err
	}

	if err := r.markProcessed(ctx, event.ID); err != nil {
		return false, err
	}

	r.logger.Printf("dispatched outbox event id=%s type=%s", dispatchEvent.ID, dispatchEvent.EventType)
	return true, nil
}

func (r *Relay) claimNextPendingEvent(ctx context.Context, reclaimBefore pgtype.Timestamptz) (db.OutboxEvent, bool, error) {
	var event db.OutboxEvent

	err := r.txRunner.WithinTx(ctx, func(store EventStore) error {
		var (
			claimed db.OutboxEvent
			err     error
		)
		if len(r.config.EventTypes) > 0 {
			claimed, err = store.ClaimNextPendingOutboxEventByTypes(ctx, db.ClaimNextPendingOutboxEventByTypesParams{
				ReclaimBefore: reclaimBefore,
				EventTypes:    r.config.EventTypes,
			})
		} else {
			claimed, err = store.ClaimNextPendingOutboxEvent(ctx, reclaimBefore)
		}
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

func (r *Relay) markProcessed(ctx context.Context, id pgtype.UUID) error {
	return r.txRunner.WithinTx(ctx, func(store EventStore) error {
		_, err := store.MarkOutboxEventAsProcessed(ctx, id)
		return err
	})
}

func (r *Relay) releaseClaim(ctx context.Context, id pgtype.UUID) error {
	return r.txRunner.WithinTx(ctx, func(store EventStore) error {
		_, err := store.ReleaseOutboxEventClaim(ctx, id)
		return err
	})
}
