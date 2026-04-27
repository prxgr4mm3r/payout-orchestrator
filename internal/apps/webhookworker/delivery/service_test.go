package delivery

import (
	"context"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/prxgr4mm3r/payout-orchestrator/internal/db"
	"github.com/prxgr4mm3r/payout-orchestrator/internal/outbox"
)

type fakeStore struct {
	createWebhookDelivery        func(ctx context.Context, arg db.CreateWebhookDeliveryParams) (db.WebhookDelivery, error)
	markWebhookDeliveryDelivered func(ctx context.Context, id pgtype.UUID) (db.WebhookDelivery, error)
	markWebhookDeliveryFailed    func(ctx context.Context, id pgtype.UUID) (db.WebhookDelivery, error)
}

func (f fakeStore) CreateWebhookDelivery(ctx context.Context, arg db.CreateWebhookDeliveryParams) (db.WebhookDelivery, error) {
	return f.createWebhookDelivery(ctx, arg)
}

func (f fakeStore) MarkWebhookDeliveryDelivered(ctx context.Context, id pgtype.UUID) (db.WebhookDelivery, error) {
	return f.markWebhookDeliveryDelivered(ctx, id)
}

func (f fakeStore) MarkWebhookDeliveryFailed(ctx context.Context, id pgtype.UUID) (db.WebhookDelivery, error) {
	return f.markWebhookDeliveryFailed(ctx, id)
}

func TestHandleEventDeliversWebhook(t *testing.T) {
	t.Parallel()

	payoutID := mustUUID(t, "8f6d6580-5dc1-43ca-bcea-b6faf36b2b32")
	clientID := mustUUID(t, "2c97a4da-38a7-46a8-9205-6482d0cfc6fb")
	deliveryID := mustUUID(t, "efb98fe4-b75f-4f1d-b9c7-794e66da2abb")

	var receivedBody []byte
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("expected POST, got %s", r.Method)
		}
		if r.Header.Get("Content-Type") != "application/json" {
			t.Fatalf("expected application/json content type, got %s", r.Header.Get("Content-Type"))
		}

		var err error
		receivedBody, err = io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("read body: %v", err)
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(server.Close)

	payload, err := outbox.MarshalPayoutResultWebhookPayload(
		payoutID.String(),
		clientID.String(),
		server.URL,
		"succeeded",
		"",
	)
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}

	created := false
	delivered := false
	service := NewService(fakeStore{
		createWebhookDelivery: func(_ context.Context, arg db.CreateWebhookDeliveryParams) (db.WebhookDelivery, error) {
			created = true
			if arg.PayoutID != payoutID {
				t.Fatalf("expected payout id %s, got %s", payoutID.String(), arg.PayoutID.String())
			}
			if arg.ClientID != clientID {
				t.Fatalf("expected client id %s, got %s", clientID.String(), arg.ClientID.String())
			}
			if arg.TargetUrl != server.URL {
				t.Fatalf("expected target url %s, got %s", server.URL, arg.TargetUrl)
			}
			if arg.Status != "pending" {
				t.Fatalf("expected pending status, got %s", arg.Status)
			}
			if arg.AttemptCount != 0 {
				t.Fatalf("expected attempt count 0, got %d", arg.AttemptCount)
			}

			return db.WebhookDelivery{ID: deliveryID}, nil
		},
		markWebhookDeliveryDelivered: func(_ context.Context, id pgtype.UUID) (db.WebhookDelivery, error) {
			delivered = true
			if id != deliveryID {
				t.Fatalf("expected delivery id %s, got %s", deliveryID.String(), id.String())
			}
			return db.WebhookDelivery{ID: id, Status: "delivered"}, nil
		},
		markWebhookDeliveryFailed: func(context.Context, pgtype.UUID) (db.WebhookDelivery, error) {
			t.Fatal("webhook delivery should not be marked failed")
			return db.WebhookDelivery{}, nil
		},
	}, server.Client(), log.New(io.Discard, "", 0))

	err = service.HandleEvent(context.Background(), outbox.Event{
		ID:        "event-1",
		EventType: outbox.EventTypePayoutResultWebhook,
		EntityID:  payoutID.String(),
		Payload:   payload,
	})
	if err != nil {
		t.Fatalf("handle event: %v", err)
	}
	if !created {
		t.Fatal("expected webhook delivery record to be created")
	}
	if !delivered {
		t.Fatal("expected webhook delivery record to be marked delivered")
	}
	if string(receivedBody) != string(payload) {
		t.Fatalf("expected delivered body %s, got %s", string(payload), string(receivedBody))
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
