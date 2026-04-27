package execution

import (
	"context"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/prxgr4mm3r/payout-orchestrator/internal/db"
)

type Store interface {
	CreateWebhookDelivery(ctx context.Context, arg db.CreateWebhookDeliveryParams) (db.WebhookDelivery, error)
	GetClientById(ctx context.Context, id pgtype.UUID) (db.Client, error)
	GetFundingSourceByClientID(ctx context.Context, arg db.GetFundingSourceByClientIDParams) (db.FundingSource, error)
	GetPayoutByClientID(ctx context.Context, arg db.GetPayoutByClientIDParams) (db.Payout, error)
	UpdatePayoutFailure(ctx context.Context, arg db.UpdatePayoutFailureParams) (db.Payout, error)
	UpdatePayoutStatus(ctx context.Context, arg db.UpdatePayoutStatusParams) (db.Payout, error)
}

type TxRunner interface {
	WithinTx(ctx context.Context, fn func(store Store) error) error
}
