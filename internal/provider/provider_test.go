package provider

import (
	"errors"
	"testing"

	payoutdomain "github.com/prxgr4mm3r/payout-orchestrator/internal/domain/payout"
)

func TestValidateResult(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		result ExecutePayoutResult
		want   error
	}{
		{
			name: "succeeded without failure reason",
			result: ExecutePayoutResult{
				Status: payoutdomain.StatusSucceeded,
			},
		},
		{
			name: "failed with failure reason",
			result: ExecutePayoutResult{
				Status:        payoutdomain.StatusFailed,
				FailureReason: "provider rejected payout",
			},
		},
		{
			name: "succeeded with failure reason",
			result: ExecutePayoutResult{
				Status:        payoutdomain.StatusSucceeded,
				FailureReason: "unexpected",
			},
			want: ErrInvalidResult,
		},
		{
			name: "failed without failure reason",
			result: ExecutePayoutResult{
				Status: payoutdomain.StatusFailed,
			},
			want: ErrInvalidResult,
		},
		{
			name: "invalid status",
			result: ExecutePayoutResult{
				Status: payoutdomain.StatusPending,
			},
			want: ErrInvalidResult,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			err := ValidateResult(tt.result)
			if !errors.Is(err, tt.want) {
				t.Fatalf("expected error %v, got %v", tt.want, err)
			}
		})
	}
}
