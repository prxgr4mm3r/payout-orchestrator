package execution

import (
	"context"
	"errors"
	"log"
	"math/big"
	"strings"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/prxgr4mm3r/payout-orchestrator/internal/db"
	payoutdomain "github.com/prxgr4mm3r/payout-orchestrator/internal/domain/payout"
	"github.com/prxgr4mm3r/payout-orchestrator/internal/outbox"
	"github.com/prxgr4mm3r/payout-orchestrator/internal/platform/pgtypeutil"
	"github.com/prxgr4mm3r/payout-orchestrator/internal/provider"
)

var (
	ErrInvalidOutboxPayload       = errors.New("invalid outbox payload")
	ErrInvalidOutboxEntity        = errors.New("invalid outbox entity")
	ErrPayoutNotReadyForExecution = errors.New("payout is not ready for execution")
	ErrUnsupportedEvent           = errors.New("unsupported event")
	ErrUnsupportedProviderResult  = errors.New("unsupported provider result")
	ErrUnsupportedNumeric         = errors.New("unsupported numeric value")
)

type Handler struct {
	txRunner TxRunner
	provider provider.PayoutProvider
	logger   *log.Logger
}

func NewHandler(txRunner TxRunner, provider provider.PayoutProvider, logger *log.Logger) *Handler {
	if logger == nil {
		logger = log.Default()
	}

	return &Handler{
		txRunner: txRunner,
		provider: provider,
		logger:   logger,
	}
}

func (h *Handler) HandleEvent(ctx context.Context, event outbox.Event) error {
	if h == nil || h.txRunner == nil || h.provider == nil {
		return errors.New("payout execution handler is not configured")
	}
	if event.EventType != outbox.EventTypeProcessPayout {
		return ErrUnsupportedEvent
	}

	payload, err := parsePayoutCreatedOutboxPayload(event.Payload)
	if err != nil {
		return err
	}

	prepared, err := h.prepareExecution(ctx, payload)
	if err != nil {
		return err
	}
	if !prepared.shouldExecute {
		return nil
	}

	result, err := h.provider.ExecutePayout(ctx, provider.ExecutePayoutInput{
		PayoutID:         prepared.payout.ID.String(),
		FundingSourceID:  prepared.fundingSource.ID.String(),
		PaymentAccountID: prepared.fundingSource.PaymentAccountID,
		Amount:           prepared.amount,
		Currency:         prepared.payout.Currency,
	})
	if err != nil {
		return err
	}

	return h.recordExecutionResult(ctx, event, prepared, result)
}

type preparedExecution struct {
	payout        db.Payout
	fundingSource db.FundingSource
	amount        string
	shouldExecute bool
}

func (h *Handler) prepareExecution(ctx context.Context, payload parsedPayoutCreatedOutboxPayload) (preparedExecution, error) {
	var prepared preparedExecution

	err := h.txRunner.WithinTx(ctx, func(store Store) error {
		if store == nil {
			return errors.New("payout execution store is not configured")
		}

		payoutRecord, fundingSource, amount, shouldExecute, err := prepareExecutionInTx(ctx, store, payload)
		if err != nil {
			return err
		}

		prepared = preparedExecution{
			payout:        payoutRecord,
			fundingSource: fundingSource,
			amount:        amount,
			shouldExecute: shouldExecute,
		}
		return nil
	})
	if err != nil {
		return preparedExecution{}, err
	}

	return prepared, nil
}

func prepareExecutionInTx(ctx context.Context, store Store, payload parsedPayoutCreatedOutboxPayload) (db.Payout, db.FundingSource, string, bool, error) {
	payoutRecord, err := store.GetPayoutByClientID(ctx, db.GetPayoutByClientIDParams{
		ClientID: payload.clientID,
		ID:       payload.payoutID,
	})
	if err != nil {
		return db.Payout{}, db.FundingSource{}, "", false, err
	}

	fundingSource, err := store.GetFundingSourceByClientID(ctx, db.GetFundingSourceByClientIDParams{
		ClientID: payload.clientID,
		ID:       payoutRecord.FundingSourceID,
	})
	if err != nil {
		return db.Payout{}, db.FundingSource{}, "", false, err
	}

	switch payoutdomain.Status(payoutRecord.Status) {
	case payoutdomain.StatusPending:
		payoutRecord, err = store.UpdatePayoutStatus(ctx, db.UpdatePayoutStatusParams{
			ID:     payoutRecord.ID,
			Status: string(payoutdomain.StatusProcessing),
		})
		if err != nil {
			return db.Payout{}, db.FundingSource{}, "", false, err
		}
	case payoutdomain.StatusProcessing:
	case payoutdomain.StatusSucceeded:
		return payoutRecord, fundingSource, "", false, nil
	default:
		return db.Payout{}, db.FundingSource{}, "", false, ErrPayoutNotReadyForExecution
	}

	amount, err := numericString(payoutRecord.Amount)
	if err != nil {
		return db.Payout{}, db.FundingSource{}, "", false, err
	}

	return payoutRecord, fundingSource, amount, true, nil
}

func (h *Handler) recordExecutionResult(ctx context.Context, event outbox.Event, prepared preparedExecution, result provider.ExecutePayoutResult) error {
	return h.txRunner.WithinTx(ctx, func(store Store) error {
		if store == nil {
			return errors.New("payout execution store is not configured")
		}

		return h.recordExecutionResultInTx(ctx, store, event, prepared, result)
	})
}

func (h *Handler) recordExecutionResultInTx(ctx context.Context, store Store, event outbox.Event, prepared preparedExecution, result provider.ExecutePayoutResult) error {
	switch result.Status {
	case payoutdomain.StatusSucceeded:
		if _, err := store.UpdatePayoutStatus(ctx, db.UpdatePayoutStatusParams{
			ID:     prepared.payout.ID,
			Status: string(payoutdomain.StatusSucceeded),
		}); err != nil {
			return err
		}

		h.logger.Printf(
			"payout execution succeeded payout_id=%s event_id=%s funding_source_id=%s",
			prepared.payout.ID.String(),
			event.ID,
			prepared.fundingSource.ID.String(),
		)
		return nil
	case payoutdomain.StatusFailed:
		if _, err := store.UpdatePayoutFailure(ctx, db.UpdatePayoutFailureParams{
			ID: prepared.payout.ID,
			FailureReason: pgtype.Text{
				String: result.FailureReason,
				Valid:  strings.TrimSpace(result.FailureReason) != "",
			},
		}); err != nil {
			return err
		}

		h.logger.Printf(
			"payout execution failed payout_id=%s event_id=%s funding_source_id=%s failure_reason=%q",
			prepared.payout.ID.String(),
			event.ID,
			prepared.fundingSource.ID.String(),
			result.FailureReason,
		)
		return nil
	default:
		return ErrUnsupportedProviderResult
	}
}

type parsedPayoutCreatedOutboxPayload struct {
	payoutID pgtype.UUID
	clientID pgtype.UUID
}

func parsePayoutCreatedOutboxPayload(raw []byte) (parsedPayoutCreatedOutboxPayload, error) {
	payload, err := outbox.UnmarshalProcessPayoutPayload(raw)
	if err != nil {
		return parsedPayoutCreatedOutboxPayload{}, ErrInvalidOutboxPayload
	}

	payoutID, err := pgtypeutil.ParseUUID(payload.PayoutID)
	if err != nil {
		return parsedPayoutCreatedOutboxPayload{}, ErrInvalidOutboxEntity
	}

	clientID, err := pgtypeutil.ParseUUID(payload.ClientID)
	if err != nil {
		return parsedPayoutCreatedOutboxPayload{}, ErrInvalidOutboxEntity
	}

	return parsedPayoutCreatedOutboxPayload{
		payoutID: payoutID,
		clientID: clientID,
	}, nil
}

func numericString(value pgtype.Numeric) (string, error) {
	if !value.Valid || value.Int == nil || value.NaN || value.InfinityModifier != pgtype.Finite {
		return "", ErrUnsupportedNumeric
	}

	sign := ""
	abs := new(big.Int).Set(value.Int)
	if abs.Sign() < 0 {
		sign = "-"
		abs.Abs(abs)
	}

	digits := abs.String()
	if value.Exp >= 0 {
		return sign + digits + strings.Repeat("0", int(value.Exp)), nil
	}

	scale := int(-value.Exp)
	if len(digits) <= scale {
		return sign + "0." + strings.Repeat("0", scale-len(digits)) + digits, nil
	}

	point := len(digits) - scale
	return sign + digits[:point] + "." + digits[point:], nil
}
