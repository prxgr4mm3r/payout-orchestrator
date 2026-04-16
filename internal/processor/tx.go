package processor

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"

	"github.com/prxgr4mm3r/payout-orchestrator/internal/db"
)

type txBeginner interface {
	Begin(ctx context.Context) (pgx.Tx, error)
}

type DBTxRunner struct {
	beginner txBeginner
	queries  *db.Queries
}

func NewDBTxRunner(beginner txBeginner, queries *db.Queries) *DBTxRunner {
	return &DBTxRunner{
		beginner: beginner,
		queries:  queries,
	}
}

func (r *DBTxRunner) WithinTx(ctx context.Context, fn func(store Store) error) error {
	if r == nil || r.beginner == nil || r.queries == nil {
		return errors.New("processor transaction runner is not configured")
	}

	tx, err := r.beginner.Begin(ctx)
	if err != nil {
		return err
	}

	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback(ctx)
		}
	}()

	if err := fn(r.queries.WithTx(tx)); err != nil {
		return err
	}

	if err := tx.Commit(ctx); err != nil {
		return err
	}

	committed = true
	return nil
}
