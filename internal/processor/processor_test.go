package processor

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/prxgr4mm3r/payout-orchestrator/internal/db"
	payoutdomain "github.com/prxgr4mm3r/payout-orchestrator/internal/domain/payout"
	"github.com/prxgr4mm3r/payout-orchestrator/internal/provider"
)

type fakeStore struct {
	claim               func(ctx context.Context, reclaimBefore pgtype.Timestamptz) (db.OutboxEvent, error)
	getFundingSource    func(ctx context.Context, arg db.GetFundingSourceByClientIDParams) (db.FundingSource, error)
	getPayout           func(ctx context.Context, arg db.GetPayoutByClientIDParams) (db.Payout, error)
	markProcessed       func(ctx context.Context, id pgtype.UUID) (db.OutboxEvent, error)
	release             func(ctx context.Context, id pgtype.UUID) (db.OutboxEvent, error)
	updatePayoutFailure func(ctx context.Context, arg db.UpdatePayoutFailureParams) (db.Payout, error)
	updatePayoutStatus  func(ctx context.Context, arg db.UpdatePayoutStatusParams) (db.Payout, error)
}

type fakeTxRunner struct {
	run func(ctx context.Context, fn func(store Store) error) error
}

type fakeProvider struct {
	execute func(ctx context.Context, input provider.ExecutePayoutInput) (provider.ExecutePayoutResult, error)
}

func (f fakeTxRunner) WithinTx(ctx context.Context, fn func(store Store) error) error {
	return f.run(ctx, fn)
}

func (f fakeStore) ClaimNextPendingOutboxEvent(ctx context.Context, reclaimBefore pgtype.Timestamptz) (db.OutboxEvent, error) {
	return f.claim(ctx, reclaimBefore)
}

func (f fakeStore) GetFundingSourceByClientID(ctx context.Context, arg db.GetFundingSourceByClientIDParams) (db.FundingSource, error) {
	return f.getFundingSource(ctx, arg)
}

func (f fakeStore) GetPayoutByClientID(ctx context.Context, arg db.GetPayoutByClientIDParams) (db.Payout, error) {
	return f.getPayout(ctx, arg)
}

func (f fakeStore) MarkOutboxEventAsProcessed(ctx context.Context, id pgtype.UUID) (db.OutboxEvent, error) {
	return f.markProcessed(ctx, id)
}

func (f fakeStore) ReleaseOutboxEventClaim(ctx context.Context, id pgtype.UUID) (db.OutboxEvent, error) {
	return f.release(ctx, id)
}

func (f fakeStore) UpdatePayoutFailure(ctx context.Context, arg db.UpdatePayoutFailureParams) (db.Payout, error) {
	return f.updatePayoutFailure(ctx, arg)
}

func (f fakeStore) UpdatePayoutStatus(ctx context.Context, arg db.UpdatePayoutStatusParams) (db.Payout, error) {
	return f.updatePayoutStatus(ctx, arg)
}

func (f fakeProvider) ExecutePayout(ctx context.Context, input provider.ExecutePayoutInput) (provider.ExecutePayoutResult, error) {
	return f.execute(ctx, input)
}

func TestRunOnceClaimsOutboxEvent(t *testing.T) {
	t.Parallel()

	eventID := mustUUID(t, "efb98fe4-b75f-4f1d-b9c7-794e66da2abb")
	handlerCalled := false
	txCalled := false

	processor := New(fakeTxRunner{
		run: func(ctx context.Context, fn func(store Store) error) error {
			txCalled = true
			return fn(fakeStore{
				claim: func(_ context.Context, reclaimBefore pgtype.Timestamptz) (db.OutboxEvent, error) {
					if !reclaimBefore.Valid || reclaimBefore.Time.IsZero() {
						t.Fatal("expected non-zero reclaim deadline")
					}

					return db.OutboxEvent{
						ID:        eventID,
						EventType: "process_payout",
						Status:    "processing",
						EntityID:  mustUUID(t, "2c97a4da-38a7-46a8-9205-6482d0cfc6fb"),
					}, nil
				},
				release: func(context.Context, pgtype.UUID) (db.OutboxEvent, error) {
					t.Fatal("release should not be called on successful claim")
					return db.OutboxEvent{}, nil
				},
			})
		},
	}, HandlerFunc(func(context.Context, Store, db.OutboxEvent) error {
		handlerCalled = true
		return nil
	}), log.New(io.Discard, "", 0), Config{ClaimTimeout: 15 * time.Second})

	claimed, err := processor.RunOnce(context.Background())
	if err != nil {
		t.Fatalf("run once: %v", err)
	}
	if !txCalled {
		t.Fatal("expected transaction runner to be used")
	}
	if !claimed {
		t.Fatal("expected outbox event to be claimed")
	}
	if !handlerCalled {
		t.Fatal("expected handler to be called")
	}
}

func TestRunOnceReturnsFalseWhenNoRowsAreAvailable(t *testing.T) {
	t.Parallel()

	handlerCalled := false

	processor := New(fakeTxRunner{
		run: func(ctx context.Context, fn func(store Store) error) error {
			return fn(fakeStore{
				claim: func(context.Context, pgtype.Timestamptz) (db.OutboxEvent, error) {
					return db.OutboxEvent{}, pgx.ErrNoRows
				},
				release: func(context.Context, pgtype.UUID) (db.OutboxEvent, error) {
					t.Fatal("release should not be called when no rows are claimed")
					return db.OutboxEvent{}, nil
				},
			})
		},
	}, HandlerFunc(func(context.Context, Store, db.OutboxEvent) error {
		handlerCalled = true
		return nil
	}), log.New(io.Discard, "", 0), Config{})

	claimed, err := processor.RunOnce(context.Background())
	if err != nil {
		t.Fatalf("run once: %v", err)
	}
	if claimed {
		t.Fatal("expected no outbox event to be claimed")
	}
	if handlerCalled {
		t.Fatal("expected handler not to be called")
	}
}

func TestRunOnceReleasesClaimWhenHandlerFails(t *testing.T) {
	t.Parallel()

	eventID := mustUUID(t, "efb98fe4-b75f-4f1d-b9c7-794e66da2abb")
	expectedErr := errors.New("handle event")
	released := false

	processor := New(fakeTxRunner{
		run: func(ctx context.Context, fn func(store Store) error) error {
			return fn(fakeStore{
				claim: func(context.Context, pgtype.Timestamptz) (db.OutboxEvent, error) {
					return db.OutboxEvent{ID: eventID}, nil
				},
				release: func(_ context.Context, id pgtype.UUID) (db.OutboxEvent, error) {
					released = true
					if id != eventID {
						t.Fatalf("expected release id %s, got %s", eventID.String(), id.String())
					}

					return db.OutboxEvent{}, nil
				},
			})
		},
	}, HandlerFunc(func(context.Context, Store, db.OutboxEvent) error {
		return expectedErr
	}), log.New(io.Discard, "", 0), Config{})

	claimed, err := processor.RunOnce(context.Background())
	if !errors.Is(err, expectedErr) {
		t.Fatalf("expected error %v, got %v", expectedErr, err)
	}
	if claimed {
		t.Fatal("expected claim to be reported as unsuccessful on handler error")
	}
	if !released {
		t.Fatal("expected claim to be released")
	}
}

func TestRunOnceReleasesClaimWhenHandlerSkips(t *testing.T) {
	t.Parallel()

	released := false

	processor := New(fakeTxRunner{
		run: func(ctx context.Context, fn func(store Store) error) error {
			return fn(fakeStore{
				claim: func(context.Context, pgtype.Timestamptz) (db.OutboxEvent, error) {
					return db.OutboxEvent{ID: mustUUID(t, "efb98fe4-b75f-4f1d-b9c7-794e66da2abb")}, nil
				},
				release: func(context.Context, pgtype.UUID) (db.OutboxEvent, error) {
					released = true
					return db.OutboxEvent{}, nil
				},
			})
		},
	}, HandlerFunc(func(context.Context, Store, db.OutboxEvent) error {
		return ErrSkipClaim
	}), log.New(io.Discard, "", 0), Config{})

	claimed, err := processor.RunOnce(context.Background())
	if err != nil {
		t.Fatalf("run once: %v", err)
	}
	if claimed {
		t.Fatal("expected skipped claim not to be reported as claimed")
	}
	if !released {
		t.Fatal("expected skipped claim to be released")
	}
}

func TestRunOnceProcessesPayoutToSuccess(t *testing.T) {
	t.Parallel()

	eventID := mustUUID(t, "efb98fe4-b75f-4f1d-b9c7-794e66da2abb")
	payoutID := mustUUID(t, "8f6d6580-5dc1-43ca-bcea-b6faf36b2b32")
	clientID := mustUUID(t, "2c97a4da-38a7-46a8-9205-6482d0cfc6fb")
	fundingSourceID := mustUUID(t, "b76e34c6-d2da-45b1-a0c1-307bc76918bd")

	payload, err := json.Marshal(payoutCreatedOutboxPayload{
		PayoutID: payoutID.String(),
		ClientID: clientID.String(),
	})
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}

	statuses := make([]string, 0, 2)
	markedProcessed := false
	providerCalled := false

	processor := New(fakeTxRunner{
		run: func(ctx context.Context, fn func(store Store) error) error {
			return fn(fakeStore{
				claim: func(_ context.Context, reclaimBefore pgtype.Timestamptz) (db.OutboxEvent, error) {
					if !reclaimBefore.Valid || reclaimBefore.Time.IsZero() {
						t.Fatal("expected non-zero reclaim deadline")
					}

					return db.OutboxEvent{
						ID:        eventID,
						EntityID:  payoutID,
						EventType: payoutCreatedOutboxEventType,
						Payload:   payload,
						Status:    "processing",
					}, nil
				},
				getPayout: func(_ context.Context, arg db.GetPayoutByClientIDParams) (db.Payout, error) {
					if arg.ClientID != clientID {
						t.Fatalf("expected client id %s, got %s", clientID.String(), arg.ClientID.String())
					}
					if arg.ID != payoutID {
						t.Fatalf("expected payout id %s, got %s", payoutID.String(), arg.ID.String())
					}

					return dbPayout(t, payoutID, clientID, fundingSourceID, "125.50", "USDC", string(payoutdomain.StatusPending), ""), nil
				},
				getFundingSource: func(_ context.Context, arg db.GetFundingSourceByClientIDParams) (db.FundingSource, error) {
					if arg.ClientID != clientID {
						t.Fatalf("expected funding source client id %s, got %s", clientID.String(), arg.ClientID.String())
					}
					if arg.ID != fundingSourceID {
						t.Fatalf("expected funding source id %s, got %s", fundingSourceID.String(), arg.ID.String())
					}

					return db.FundingSource{
						ID:               fundingSourceID,
						ClientID:         clientID,
						PaymentAccountID: "acct_123",
					}, nil
				},
				updatePayoutStatus: func(_ context.Context, arg db.UpdatePayoutStatusParams) (db.Payout, error) {
					if arg.ID != payoutID {
						t.Fatalf("expected update payout id %s, got %s", payoutID.String(), arg.ID.String())
					}

					statuses = append(statuses, arg.Status)
					return dbPayout(t, payoutID, clientID, fundingSourceID, "125.50", "USDC", arg.Status, ""), nil
				},
				markProcessed: func(_ context.Context, id pgtype.UUID) (db.OutboxEvent, error) {
					if id != eventID {
						t.Fatalf("expected processed event id %s, got %s", eventID.String(), id.String())
					}

					markedProcessed = true
					return db.OutboxEvent{ID: eventID, Status: "processed"}, nil
				},
				release: func(context.Context, pgtype.UUID) (db.OutboxEvent, error) {
					t.Fatal("release should not be called on successful execution")
					return db.OutboxEvent{}, nil
				},
			})
		},
	}, NewExecutionHandler(fakeProvider{
		execute: func(_ context.Context, input provider.ExecutePayoutInput) (provider.ExecutePayoutResult, error) {
			providerCalled = true
			if input.PayoutID != payoutID.String() {
				t.Fatalf("expected payout id %s, got %s", payoutID.String(), input.PayoutID)
			}
			if input.FundingSourceID != fundingSourceID.String() {
				t.Fatalf("expected funding source id %s, got %s", fundingSourceID.String(), input.FundingSourceID)
			}
			if input.PaymentAccountID != "acct_123" {
				t.Fatalf("expected payment account acct_123, got %s", input.PaymentAccountID)
			}
			if input.Amount != "125.50" {
				t.Fatalf("expected amount 125.50, got %s", input.Amount)
			}
			if input.Currency != "USDC" {
				t.Fatalf("expected currency USDC, got %s", input.Currency)
			}

			return provider.ExecutePayoutResult{Status: payoutdomain.StatusSucceeded}, nil
		},
	}, log.New(io.Discard, "", 0)), log.New(io.Discard, "", 0), Config{ClaimTimeout: 15 * time.Second})

	claimed, err := processor.RunOnce(context.Background())
	if err != nil {
		t.Fatalf("run once: %v", err)
	}
	if !claimed {
		t.Fatal("expected outbox event to be claimed")
	}
	if !providerCalled {
		t.Fatal("expected provider to be called")
	}
	if !markedProcessed {
		t.Fatal("expected outbox event to be marked processed")
	}
	if len(statuses) != 2 {
		t.Fatalf("expected 2 payout status updates, got %d", len(statuses))
	}
	if statuses[0] != string(payoutdomain.StatusProcessing) {
		t.Fatalf("expected first status update to processing, got %s", statuses[0])
	}
	if statuses[1] != string(payoutdomain.StatusSucceeded) {
		t.Fatalf("expected second status update to succeeded, got %s", statuses[1])
	}
}

func TestRunOncePersistsFailedPayoutOutcome(t *testing.T) {
	t.Parallel()

	eventID := mustUUID(t, "efb98fe4-b75f-4f1d-b9c7-794e66da2abb")
	payoutID := mustUUID(t, "8f6d6580-5dc1-43ca-bcea-b6faf36b2b32")
	clientID := mustUUID(t, "2c97a4da-38a7-46a8-9205-6482d0cfc6fb")
	fundingSourceID := mustUUID(t, "b76e34c6-d2da-45b1-a0c1-307bc76918bd")

	payload, err := json.Marshal(payoutCreatedOutboxPayload{
		PayoutID: payoutID.String(),
		ClientID: clientID.String(),
	})
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}

	var failedReason string
	markedProcessed := false
	providerCalled := false

	processor := New(fakeTxRunner{
		run: func(ctx context.Context, fn func(store Store) error) error {
			return fn(fakeStore{
				claim: func(context.Context, pgtype.Timestamptz) (db.OutboxEvent, error) {
					return db.OutboxEvent{
						ID:        eventID,
						EntityID:  payoutID,
						EventType: payoutCreatedOutboxEventType,
						Payload:   payload,
						Status:    "processing",
					}, nil
				},
				getPayout: func(_ context.Context, arg db.GetPayoutByClientIDParams) (db.Payout, error) {
					return dbPayout(t, payoutID, clientID, fundingSourceID, "125.50", "USDC", string(payoutdomain.StatusPending), ""), nil
				},
				getFundingSource: func(_ context.Context, arg db.GetFundingSourceByClientIDParams) (db.FundingSource, error) {
					return db.FundingSource{
						ID:               fundingSourceID,
						ClientID:         clientID,
						PaymentAccountID: "acct_123",
					}, nil
				},
				updatePayoutStatus: func(_ context.Context, arg db.UpdatePayoutStatusParams) (db.Payout, error) {
					return dbPayout(t, payoutID, clientID, fundingSourceID, "125.50", "USDC", arg.Status, ""), nil
				},
				updatePayoutFailure: func(_ context.Context, arg db.UpdatePayoutFailureParams) (db.Payout, error) {
					if arg.ID != payoutID {
						t.Fatalf("expected failed payout id %s, got %s", payoutID.String(), arg.ID.String())
					}
					failedReason = arg.FailureReason.String
					return dbPayout(t, payoutID, clientID, fundingSourceID, "125.50", "USDC", string(payoutdomain.StatusFailed), arg.FailureReason.String), nil
				},
				markProcessed: func(_ context.Context, id pgtype.UUID) (db.OutboxEvent, error) {
					if id != eventID {
						t.Fatalf("expected processed event id %s, got %s", eventID.String(), id.String())
					}
					markedProcessed = true
					return db.OutboxEvent{ID: eventID, Status: "processed"}, nil
				},
				release: func(context.Context, pgtype.UUID) (db.OutboxEvent, error) {
					t.Fatal("release should not be called on failed outcome")
					return db.OutboxEvent{}, nil
				},
			})
		},
	}, NewExecutionHandler(fakeProvider{
		execute: func(_ context.Context, input provider.ExecutePayoutInput) (provider.ExecutePayoutResult, error) {
			providerCalled = true
			return provider.ExecutePayoutResult{
				Status:        payoutdomain.StatusFailed,
				FailureReason: "provider rejected payout",
			}, nil
		},
	}, log.New(io.Discard, "", 0)), log.New(io.Discard, "", 0), Config{ClaimTimeout: 15 * time.Second})

	claimed, err := processor.RunOnce(context.Background())
	if err != nil {
		t.Fatalf("run once: %v", err)
	}
	if !claimed {
		t.Fatal("expected outbox event to be claimed")
	}
	if !providerCalled {
		t.Fatal("expected provider to be called")
	}
	if failedReason != "provider rejected payout" {
		t.Fatalf("expected failed reason to be persisted, got %q", failedReason)
	}
	if !markedProcessed {
		t.Fatal("expected outbox event to be marked processed")
	}
}

func mustUUID(t *testing.T, raw string) pgtype.UUID {
	t.Helper()

	var id pgtype.UUID
	if err := id.Scan(raw); err != nil {
		t.Fatalf("scan uuid %q: %v", raw, err)
	}

	return id
}

func dbPayout(t *testing.T, payoutID, clientID, fundingSourceID pgtype.UUID, amount, currency, status, failureReason string) db.Payout {
	t.Helper()

	now := pgtype.Timestamptz{Time: time.Date(2026, 4, 17, 12, 0, 0, 0, time.UTC), Valid: true}

	return db.Payout{
		ID:              payoutID,
		ClientID:        clientID,
		FundingSourceID: fundingSourceID,
		Amount:          numericFromDecimal(t, amount),
		Currency:        currency,
		Status:          status,
		FailureReason:   pgtype.Text{String: failureReason, Valid: failureReason != ""},
		CreatedAt:       now,
		UpdatedAt:       now,
	}
}

func numericFromDecimal(t *testing.T, raw string) pgtype.Numeric {
	t.Helper()

	var numeric pgtype.Numeric
	if err := numeric.Scan(raw); err != nil {
		t.Fatalf("scan numeric %q: %v", raw, err)
	}

	return numeric
}
