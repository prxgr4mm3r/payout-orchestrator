package provider

import (
	"context"
	"errors"
	"strings"

	payoutdomain "github.com/prxgr4mm3r/payout-orchestrator/internal/domain/payout"
)

var ErrInvalidResult = errors.New("invalid payout provider result")

type ExecutePayoutInput struct {
	PayoutID         string
	FundingSourceID  string
	PaymentAccountID string
	Amount           string
	Currency         string
}

type ExecutePayoutResult struct {
	Status        payoutdomain.Status
	FailureReason string
}

type PayoutProvider interface {
	ExecutePayout(ctx context.Context, input ExecutePayoutInput) (ExecutePayoutResult, error)
}

func ValidateResult(result ExecutePayoutResult) error {
	switch result.Status {
	case payoutdomain.StatusSucceeded:
		if strings.TrimSpace(result.FailureReason) != "" {
			return ErrInvalidResult
		}
	case payoutdomain.StatusFailed:
		if strings.TrimSpace(result.FailureReason) == "" {
			return ErrInvalidResult
		}
	default:
		return ErrInvalidResult
	}

	return nil
}
