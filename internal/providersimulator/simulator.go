package providersimulator

import (
	"context"
	"errors"
	"strings"

	payoutdomain "github.com/prxgr4mm3r/payout-orchestrator/internal/domain/payout"
	"github.com/prxgr4mm3r/payout-orchestrator/internal/provider"
)

var ErrInvalidOutcome = errors.New("invalid provider simulator outcome")

type Outcome string

const (
	OutcomeSucceeded Outcome = "succeeded"
	OutcomeFailed    Outcome = "failed"
)

type Config struct {
	Outcome       Outcome
	FailureReason string
}

type Simulator struct {
	config Config
}

func New(config Config) *Simulator {
	return &Simulator{config: config}
}

func (s *Simulator) ExecutePayout(ctx context.Context, input provider.ExecutePayoutInput) (provider.ExecutePayoutResult, error) {
	if err := ctx.Err(); err != nil {
		return provider.ExecutePayoutResult{}, err
	}

	result, err := s.resultFor(input)
	if err != nil {
		return provider.ExecutePayoutResult{}, err
	}

	if err := provider.ValidateResult(result); err != nil {
		return provider.ExecutePayoutResult{}, err
	}

	return result, nil
}

func (s *Simulator) resultFor(input provider.ExecutePayoutInput) (provider.ExecutePayoutResult, error) {
	outcome := s.config.Outcome
	if outcome == "" {
		outcome = outcomeFromInput(input)
	}

	switch outcome {
	case OutcomeSucceeded:
		return provider.ExecutePayoutResult{
			Status: payoutdomain.StatusSucceeded,
		}, nil
	case OutcomeFailed:
		reason := strings.TrimSpace(s.config.FailureReason)
		if reason == "" {
			reason = "provider simulator rejected payout"
		}

		return provider.ExecutePayoutResult{
			Status:        payoutdomain.StatusFailed,
			FailureReason: reason,
		}, nil
	default:
		return provider.ExecutePayoutResult{}, ErrInvalidOutcome
	}
}

func outcomeFromInput(input provider.ExecutePayoutInput) Outcome {
	accountID := strings.ToLower(strings.TrimSpace(input.PaymentAccountID))
	if strings.HasPrefix(accountID, "fail") {
		return OutcomeFailed
	}

	return OutcomeSucceeded
}
