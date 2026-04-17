package processor

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"math/big"
	"strings"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/prxgr4mm3r/payout-orchestrator/internal/db"
	payoutdomain "github.com/prxgr4mm3r/payout-orchestrator/internal/domain/payout"
	"github.com/prxgr4mm3r/payout-orchestrator/internal/platform/pgtypeutil"
	"github.com/prxgr4mm3r/payout-orchestrator/internal/provider"
)

const payoutCreatedOutboxEventType = "process_payout"

var (
	ErrInvalidOutboxPayload       = errors.New("invalid outbox payload")
	ErrInvalidOutboxEntity        = errors.New("invalid outbox entity")
	ErrPayoutNotReadyForExecution = errors.New("payout is not ready for execution")
	ErrUnsupportedProviderResult  = errors.New("unsupported provider result")
	ErrUnsupportedNumeric         = errors.New("unsupported numeric value")
)

type payoutCreatedOutboxPayload struct {
	PayoutID string `json:"payout_id"`
	ClientID string `json:"client_id"`
}

type ExecutionHandler struct {
	provider provider.PayoutProvider
	logger   *log.Logger
}

func NewExecutionHandler(provider provider.PayoutProvider, logger *log.Logger) *ExecutionHandler {
	if logger == nil {
		logger = log.Default()
	}

	return &ExecutionHandler{
		provider: provider,
		logger:   logger,
	}
}

func (h *ExecutionHandler) HandleOutboxEvent(ctx context.Context, store Store, event db.OutboxEvent) error {
	if h == nil || h.provider == nil {
		return errors.New("processor execution handler is not configured")
	}
	if event.EventType != payoutCreatedOutboxEventType {
		return ErrSkipClaim
	}

	payload, err := parsePayoutCreatedOutboxPayload(event.Payload)
	if err != nil {
		return err
	}

	payoutRecord, err := store.GetPayoutByClientID(ctx, db.GetPayoutByClientIDParams{
		ClientID: payload.clientID,
		ID:       payload.payoutID,
	})
	if err != nil {
		return err
	}

	fundingSource, err := store.GetFundingSourceByClientID(ctx, db.GetFundingSourceByClientIDParams{
		ClientID: payload.clientID,
		ID:       payoutRecord.FundingSourceID,
	})
	if err != nil {
		return err
	}

	switch payoutdomain.Status(payoutRecord.Status) {
	case payoutdomain.StatusPending:
		if _, err := store.UpdatePayoutStatus(ctx, db.UpdatePayoutStatusParams{
			ID:     payoutRecord.ID,
			Status: string(payoutdomain.StatusProcessing),
		}); err != nil {
			return err
		}
	case payoutdomain.StatusProcessing:
	case payoutdomain.StatusSucceeded:
		_, err := store.MarkOutboxEventAsProcessed(ctx, event.ID)
		return err
	default:
		return ErrPayoutNotReadyForExecution
	}

	amount, err := numericString(payoutRecord.Amount)
	if err != nil {
		return err
	}

	result, err := h.provider.ExecutePayout(ctx, provider.ExecutePayoutInput{
		PayoutID:         payoutRecord.ID.String(),
		FundingSourceID:  fundingSource.ID.String(),
		PaymentAccountID: fundingSource.PaymentAccountID,
		Amount:           amount,
		Currency:         payoutRecord.Currency,
	})
	if err != nil {
		return err
	}

	switch result.Status {
	case payoutdomain.StatusSucceeded:
		if _, err := store.UpdatePayoutStatus(ctx, db.UpdatePayoutStatusParams{
			ID:     payoutRecord.ID,
			Status: string(payoutdomain.StatusSucceeded),
		}); err != nil {
			return err
		}

		if _, err := store.MarkOutboxEventAsProcessed(ctx, event.ID); err != nil {
			return err
		}

		h.logger.Printf(
			"payout execution succeeded payout_id=%s outbox_event_id=%s funding_source_id=%s",
			payoutRecord.ID.String(),
			event.ID.String(),
			fundingSource.ID.String(),
		)
		return nil
	case payoutdomain.StatusFailed:
		if _, err := store.UpdatePayoutFailure(ctx, db.UpdatePayoutFailureParams{
			ID: payoutRecord.ID,
			FailureReason: pgtype.Text{
				String: result.FailureReason,
				Valid:  strings.TrimSpace(result.FailureReason) != "",
			},
		}); err != nil {
			return err
		}

		if _, err := store.MarkOutboxEventAsProcessed(ctx, event.ID); err != nil {
			return err
		}

		h.logger.Printf(
			"payout execution failed payout_id=%s outbox_event_id=%s funding_source_id=%s failure_reason=%q",
			payoutRecord.ID.String(),
			event.ID.String(),
			fundingSource.ID.String(),
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
	var payload payoutCreatedOutboxPayload
	if err := json.Unmarshal(raw, &payload); err != nil {
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
