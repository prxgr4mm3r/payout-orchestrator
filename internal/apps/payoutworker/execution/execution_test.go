package execution

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/prxgr4mm3r/payout-orchestrator/internal/db"
	payoutdomain "github.com/prxgr4mm3r/payout-orchestrator/internal/domain/payout"
	"github.com/prxgr4mm3r/payout-orchestrator/internal/outbox"
	"github.com/prxgr4mm3r/payout-orchestrator/internal/provider"
)

type fakeStore struct {
	createWebhookDelivery func(ctx context.Context, arg db.CreateWebhookDeliveryParams) (db.WebhookDelivery, error)
	getClient             func(ctx context.Context, id pgtype.UUID) (db.Client, error)
	getFundingSource      func(ctx context.Context, arg db.GetFundingSourceByClientIDParams) (db.FundingSource, error)
	getPayout             func(ctx context.Context, arg db.GetPayoutByClientIDParams) (db.Payout, error)
	updatePayoutFailure   func(ctx context.Context, arg db.UpdatePayoutFailureParams) (db.Payout, error)
	updatePayoutStatus    func(ctx context.Context, arg db.UpdatePayoutStatusParams) (db.Payout, error)
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

func (f fakeStore) GetFundingSourceByClientID(ctx context.Context, arg db.GetFundingSourceByClientIDParams) (db.FundingSource, error) {
	return f.getFundingSource(ctx, arg)
}

func (f fakeStore) GetPayoutByClientID(ctx context.Context, arg db.GetPayoutByClientIDParams) (db.Payout, error) {
	return f.getPayout(ctx, arg)
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

func (f fakeStore) CreateWebhookDelivery(ctx context.Context, arg db.CreateWebhookDeliveryParams) (db.WebhookDelivery, error) {
	return f.createWebhookDelivery(ctx, arg)
}

func (f fakeStore) GetClientById(ctx context.Context, id pgtype.UUID) (db.Client, error) {
	return f.getClient(ctx, id)
}

func TestHandleEventProcessesPayoutToSuccess(t *testing.T) {
	t.Parallel()

	eventID := "efb98fe4-b75f-4f1d-b9c7-794e66da2abb"
	payoutID := mustUUID(t, "8f6d6580-5dc1-43ca-bcea-b6faf36b2b32")
	clientID := mustUUID(t, "2c97a4da-38a7-46a8-9205-6482d0cfc6fb")
	fundingSourceID := mustUUID(t, "b76e34c6-d2da-45b1-a0c1-307bc76918bd")

	payload, err := outbox.MarshalProcessPayoutPayload(payoutID.String(), clientID.String())
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}

	statuses := make([]string, 0, 2)
	providerCalled := false
	webhookDeliveryCreated := false
	inTx := false

	handler := NewHandler(fakeTxRunner{
		run: func(ctx context.Context, fn func(store Store) error) error {
			inTx = true
			defer func() {
				inTx = false
			}()

			return fn(fakeStore{
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
				getClient: func(_ context.Context, id pgtype.UUID) (db.Client, error) {
					if id != clientID {
						t.Fatalf("expected client id %s, got %s", clientID.String(), id.String())
					}

					return db.Client{
						ID:         clientID,
						WebhookUrl: pgtype.Text{String: " https://example.com/webhooks/payouts ", Valid: true},
					}, nil
				},
				createWebhookDelivery: func(_ context.Context, arg db.CreateWebhookDeliveryParams) (db.WebhookDelivery, error) {
					webhookDeliveryCreated = true
					if arg.PayoutID != payoutID {
						t.Fatalf("expected webhook payout id %s, got %s", payoutID.String(), arg.PayoutID.String())
					}
					if arg.ClientID != clientID {
						t.Fatalf("expected webhook client id %s, got %s", clientID.String(), arg.ClientID.String())
					}
					if arg.TargetUrl != "https://example.com/webhooks/payouts" {
						t.Fatalf("expected trimmed webhook target url, got %q", arg.TargetUrl)
					}
					if arg.Status != "pending" {
						t.Fatalf("expected webhook status pending, got %s", arg.Status)
					}
					if arg.AttemptCount != 0 {
						t.Fatalf("expected webhook attempt count 0, got %d", arg.AttemptCount)
					}

					var payload outbox.PayoutResultWebhookPayload
					if err := json.Unmarshal(arg.Payload, &payload); err != nil {
						t.Fatalf("unmarshal webhook payload: %v", err)
					}
					if payload.EventType != outbox.EventTypePayoutResultWebhook {
						t.Fatalf("expected webhook event type %s, got %s", outbox.EventTypePayoutResultWebhook, payload.EventType)
					}
					if payload.PayoutID != payoutID.String() {
						t.Fatalf("expected payload payout id %s, got %s", payoutID.String(), payload.PayoutID)
					}
					if payload.ClientID != clientID.String() {
						t.Fatalf("expected payload client id %s, got %s", clientID.String(), payload.ClientID)
					}
					if payload.Status != string(payoutdomain.StatusSucceeded) {
						t.Fatalf("expected payload status succeeded, got %s", payload.Status)
					}
					if payload.FailureReason != "" {
						t.Fatalf("expected empty failure reason, got %q", payload.FailureReason)
					}

					return db.WebhookDelivery{}, nil
				},
				updatePayoutStatus: func(_ context.Context, arg db.UpdatePayoutStatusParams) (db.Payout, error) {
					if arg.ID != payoutID {
						t.Fatalf("expected update payout id %s, got %s", payoutID.String(), arg.ID.String())
					}

					statuses = append(statuses, arg.Status)
					return dbPayout(t, payoutID, clientID, fundingSourceID, "125.50", "USDC", arg.Status, ""), nil
				},
				updatePayoutFailure: func(context.Context, db.UpdatePayoutFailureParams) (db.Payout, error) {
					t.Fatal("failure should not be persisted on successful execution")
					return db.Payout{}, nil
				},
			})
		},
	}, fakeProvider{
		execute: func(_ context.Context, input provider.ExecutePayoutInput) (provider.ExecutePayoutResult, error) {
			providerCalled = true
			if inTx {
				t.Fatal("provider should be called outside the database transaction")
			}
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
	}, log.New(io.Discard, "", 0))

	err = handler.HandleEvent(context.Background(), outbox.Event{
		ID:        eventID,
		EventType: outbox.EventTypeProcessPayout,
		EntityID:  payoutID.String(),
		Payload:   payload,
	})
	if err != nil {
		t.Fatalf("handle event: %v", err)
	}
	if !providerCalled {
		t.Fatal("expected provider to be called")
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
	if !webhookDeliveryCreated {
		t.Fatal("expected webhook delivery to be created")
	}
}

func TestHandleEventPersistsFailedPayoutOutcome(t *testing.T) {
	t.Parallel()

	payoutID := mustUUID(t, "8f6d6580-5dc1-43ca-bcea-b6faf36b2b32")
	clientID := mustUUID(t, "2c97a4da-38a7-46a8-9205-6482d0cfc6fb")
	fundingSourceID := mustUUID(t, "b76e34c6-d2da-45b1-a0c1-307bc76918bd")

	payload, err := outbox.MarshalProcessPayoutPayload(payoutID.String(), clientID.String())
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}

	var failedReason string
	var webhookPayload outbox.PayoutResultWebhookPayload
	inTx := false

	handler := NewHandler(fakeTxRunner{
		run: func(ctx context.Context, fn func(store Store) error) error {
			inTx = true
			defer func() {
				inTx = false
			}()

			return fn(fakeStore{
				getPayout: func(context.Context, db.GetPayoutByClientIDParams) (db.Payout, error) {
					return dbPayout(t, payoutID, clientID, fundingSourceID, "125.50", "USDC", string(payoutdomain.StatusPending), ""), nil
				},
				getFundingSource: func(context.Context, db.GetFundingSourceByClientIDParams) (db.FundingSource, error) {
					return db.FundingSource{
						ID:               fundingSourceID,
						ClientID:         clientID,
						PaymentAccountID: "acct_123",
					}, nil
				},
				getClient: func(context.Context, pgtype.UUID) (db.Client, error) {
					return db.Client{
						ID:         clientID,
						WebhookUrl: pgtype.Text{String: "https://example.com/webhooks/payouts", Valid: true},
					}, nil
				},
				createWebhookDelivery: func(_ context.Context, arg db.CreateWebhookDeliveryParams) (db.WebhookDelivery, error) {
					if err := json.Unmarshal(arg.Payload, &webhookPayload); err != nil {
						t.Fatalf("unmarshal webhook payload: %v", err)
					}
					return db.WebhookDelivery{}, nil
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
			})
		},
	}, fakeProvider{
		execute: func(context.Context, provider.ExecutePayoutInput) (provider.ExecutePayoutResult, error) {
			if inTx {
				t.Fatal("provider should be called outside the database transaction")
			}

			return provider.ExecutePayoutResult{
				Status:        payoutdomain.StatusFailed,
				FailureReason: "provider rejected payout",
			}, nil
		},
	}, log.New(io.Discard, "", 0))

	err = handler.HandleEvent(context.Background(), outbox.Event{
		ID:        "efb98fe4-b75f-4f1d-b9c7-794e66da2abb",
		EventType: outbox.EventTypeProcessPayout,
		EntityID:  payoutID.String(),
		Payload:   payload,
	})
	if err != nil {
		t.Fatalf("handle event: %v", err)
	}
	if failedReason != "provider rejected payout" {
		t.Fatalf("expected failure reason to be persisted, got %q", failedReason)
	}
	if webhookPayload.Status != string(payoutdomain.StatusFailed) {
		t.Fatalf("expected webhook payload status failed, got %s", webhookPayload.Status)
	}
	if webhookPayload.FailureReason != "provider rejected payout" {
		t.Fatalf("expected webhook failure reason, got %q", webhookPayload.FailureReason)
	}
}

func TestHandleEventRejectsUnsupportedType(t *testing.T) {
	t.Parallel()

	handler := NewHandler(fakeTxRunner{
		run: func(context.Context, func(store Store) error) error {
			t.Fatal("transaction runner should not be used for unsupported events")
			return nil
		},
	}, fakeProvider{}, log.New(io.Discard, "", 0))

	err := handler.HandleEvent(context.Background(), outbox.Event{
		ID:        "efb98fe4-b75f-4f1d-b9c7-794e66da2abb",
		EventType: "unsupported",
	})
	if !errors.Is(err, ErrUnsupportedEvent) {
		t.Fatalf("expected ErrUnsupportedEvent, got %v", err)
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
