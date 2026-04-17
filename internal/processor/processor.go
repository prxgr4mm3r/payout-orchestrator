package processor

import (
	"context"
	"errors"
	"log"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/prxgr4mm3r/payout-orchestrator/internal/db"
)

var ErrSkipClaim = errors.New("skip outbox claim")

type Store interface {
	ClaimNextPendingOutboxEvent(ctx context.Context, reclaimBefore pgtype.Timestamptz) (db.OutboxEvent, error)
	GetFundingSourceByClientID(ctx context.Context, arg db.GetFundingSourceByClientIDParams) (db.FundingSource, error)
	GetPayoutByClientID(ctx context.Context, arg db.GetPayoutByClientIDParams) (db.Payout, error)
	MarkOutboxEventAsProcessed(ctx context.Context, id pgtype.UUID) (db.OutboxEvent, error)
	ReleaseOutboxEventClaim(ctx context.Context, id pgtype.UUID) (db.OutboxEvent, error)
	UpdatePayoutStatus(ctx context.Context, arg db.UpdatePayoutStatusParams) (db.Payout, error)
}

type TxRunner interface {
	WithinTx(ctx context.Context, fn func(store Store) error) error
}

type Handler interface {
	HandleOutboxEvent(ctx context.Context, store Store, event db.OutboxEvent) error
}

type HandlerFunc func(ctx context.Context, store Store, event db.OutboxEvent) error

func (f HandlerFunc) HandleOutboxEvent(ctx context.Context, store Store, event db.OutboxEvent) error {
	return f(ctx, store, event)
}

type Config struct {
	PollInterval time.Duration
	ClaimTimeout time.Duration
}

type Processor struct {
	txRunner TxRunner
	handler  Handler
	logger   *log.Logger
	config   Config
}

func New(txRunner TxRunner, handler Handler, logger *log.Logger, config Config) *Processor {
	if logger == nil {
		logger = log.Default()
	}

	if config.PollInterval <= 0 {
		config.PollInterval = time.Second
	}
	if config.ClaimTimeout <= 0 {
		config.ClaimTimeout = 30 * time.Second
	}

	return &Processor{
		txRunner: txRunner,
		handler:  handler,
		logger:   logger,
		config:   config,
	}
}

func (p *Processor) Run(ctx context.Context) error {
	if p == nil || p.txRunner == nil || p.handler == nil {
		return errors.New("processor is not configured")
	}

	ticker := time.NewTicker(p.config.PollInterval)
	defer ticker.Stop()

	for {
		claimed, err := p.RunOnce(ctx)
		if err != nil {
			return err
		}
		if claimed {
			continue
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}

func (p *Processor) RunOnce(ctx context.Context) (bool, error) {
	if p == nil || p.txRunner == nil || p.handler == nil {
		return false, errors.New("processor is not configured")
	}

	var claimed bool
	reclaimBefore := pgtype.Timestamptz{
		Time:  time.Now().Add(-p.config.ClaimTimeout),
		Valid: true,
	}

	err := p.txRunner.WithinTx(ctx, func(store Store) error {
		event, err := store.ClaimNextPendingOutboxEvent(ctx, reclaimBefore)
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		if err != nil {
			return err
		}

		claimed = true

		if err := p.handler.HandleOutboxEvent(ctx, store, event); err != nil {
			if _, releaseErr := store.ReleaseOutboxEventClaim(ctx, event.ID); releaseErr != nil {
				return releaseErr
			}

			if errors.Is(err, ErrSkipClaim) {
				p.logger.Printf("released outbox event claim id=%s", event.ID.String())
				claimed = false
				return nil
			}

			return err
		}

		p.logger.Printf("claimed outbox event id=%s type=%s", event.ID.String(), event.EventType)
		return nil
	})
	if err != nil {
		return false, err
	}

	return claimed, nil
}
