package processor

import (
	"context"

	"github.com/prxgr4mm3r/payout-orchestrator/internal/db"
)

type Store interface {
	GetFundingSourceByClientID(ctx context.Context, arg db.GetFundingSourceByClientIDParams) (db.FundingSource, error)
	GetPayoutByClientID(ctx context.Context, arg db.GetPayoutByClientIDParams) (db.Payout, error)
	UpdatePayoutFailure(ctx context.Context, arg db.UpdatePayoutFailureParams) (db.Payout, error)
	UpdatePayoutStatus(ctx context.Context, arg db.UpdatePayoutStatusParams) (db.Payout, error)
}

type TxRunner interface {
	WithinTx(ctx context.Context, fn func(store Store) error) error
}
