package payout

import "testing"

func TestStatusValid(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		status Status
		want   bool
	}{
		{name: "pending", status: StatusPending, want: true},
		{name: "processing", status: StatusProcessing, want: true},
		{name: "succeeded", status: StatusSucceeded, want: true},
		{name: "failed", status: StatusFailed, want: true},
		{name: "unknown", status: Status("unknown"), want: false},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := tt.status.Valid(); got != tt.want {
				t.Fatalf("expected %t, got %t", tt.want, got)
			}
		})
	}
}

func TestCanTransition(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		from Status
		to   Status
		want bool
	}{
		{name: "pending to processing", from: StatusPending, to: StatusProcessing, want: true},
		{name: "processing to succeeded", from: StatusProcessing, to: StatusSucceeded, want: true},
		{name: "processing to failed", from: StatusProcessing, to: StatusFailed, want: true},
		{name: "pending to succeeded", from: StatusPending, to: StatusSucceeded, want: false},
		{name: "pending to failed", from: StatusPending, to: StatusFailed, want: false},
		{name: "processing to pending", from: StatusProcessing, to: StatusPending, want: false},
		{name: "succeeded to failed", from: StatusSucceeded, to: StatusFailed, want: false},
		{name: "failed to processing", from: StatusFailed, to: StatusProcessing, want: false},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := CanTransition(tt.from, tt.to); got != tt.want {
				t.Fatalf("expected %t, got %t", tt.want, got)
			}
		})
	}
}
