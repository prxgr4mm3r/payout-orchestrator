package providersimulator

import (
	"context"
	"errors"
	"testing"

	payoutdomain "github.com/prxgr4mm3r/payout-orchestrator/internal/domain/payout"
	"github.com/prxgr4mm3r/payout-orchestrator/internal/provider"
)

func TestExecutePayoutReturnsSuccessByDefault(t *testing.T) {
	t.Parallel()

	simulator := New(Config{})

	result, err := simulator.ExecutePayout(context.Background(), provider.ExecutePayoutInput{
		PayoutID:         "payout-1",
		FundingSourceID:  "source-1",
		PaymentAccountID: "acct-1",
		Amount:           "125.50",
		Currency:         "USDC",
	})
	if err != nil {
		t.Fatalf("execute payout: %v", err)
	}
	if result.Status != payoutdomain.StatusSucceeded {
		t.Fatalf("expected status %s, got %s", payoutdomain.StatusSucceeded, result.Status)
	}
	if result.FailureReason != "" {
		t.Fatalf("expected empty failure reason, got %q", result.FailureReason)
	}
}

func TestExecutePayoutReturnsFailureForFailPrefixedAccount(t *testing.T) {
	t.Parallel()

	simulator := New(Config{})

	result, err := simulator.ExecutePayout(context.Background(), provider.ExecutePayoutInput{
		PaymentAccountID: "fail-account",
	})
	if err != nil {
		t.Fatalf("execute payout: %v", err)
	}
	if result.Status != payoutdomain.StatusFailed {
		t.Fatalf("expected status %s, got %s", payoutdomain.StatusFailed, result.Status)
	}
	if result.FailureReason != "provider simulator rejected payout" {
		t.Fatalf("expected default failure reason, got %q", result.FailureReason)
	}
}

func TestExecutePayoutUsesConfiguredFailure(t *testing.T) {
	t.Parallel()

	simulator := New(Config{
		Outcome:       OutcomeFailed,
		FailureReason: "insufficient simulator funds",
	})

	result, err := simulator.ExecutePayout(context.Background(), provider.ExecutePayoutInput{
		PaymentAccountID: "acct-1",
	})
	if err != nil {
		t.Fatalf("execute payout: %v", err)
	}
	if result.Status != payoutdomain.StatusFailed {
		t.Fatalf("expected status %s, got %s", payoutdomain.StatusFailed, result.Status)
	}
	if result.FailureReason != "insufficient simulator funds" {
		t.Fatalf("expected configured failure reason, got %q", result.FailureReason)
	}
}

func TestExecutePayoutUsesConfiguredSuccess(t *testing.T) {
	t.Parallel()

	simulator := New(Config{
		Outcome: OutcomeSucceeded,
	})

	result, err := simulator.ExecutePayout(context.Background(), provider.ExecutePayoutInput{
		PaymentAccountID: "fail-account",
	})
	if err != nil {
		t.Fatalf("execute payout: %v", err)
	}
	if result.Status != payoutdomain.StatusSucceeded {
		t.Fatalf("expected status %s, got %s", payoutdomain.StatusSucceeded, result.Status)
	}
	if result.FailureReason != "" {
		t.Fatalf("expected empty failure reason, got %q", result.FailureReason)
	}
}

func TestExecutePayoutRejectsUnknownOutcome(t *testing.T) {
	t.Parallel()

	simulator := New(Config{
		Outcome: Outcome("unknown"),
	})

	_, err := simulator.ExecutePayout(context.Background(), provider.ExecutePayoutInput{})
	if !errors.Is(err, ErrInvalidOutcome) {
		t.Fatalf("expected ErrInvalidOutcome, got %v", err)
	}
}

func TestExecutePayoutReturnsContextError(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	simulator := New(Config{})

	_, err := simulator.ExecutePayout(ctx, provider.ExecutePayoutInput{})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled, got %v", err)
	}
}
