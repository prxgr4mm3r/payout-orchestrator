package rabbitmq

import (
	"context"
	"testing"

	"github.com/prxgr4mm3r/payout-orchestrator/internal/outbox"
)

type fakePublisher struct {
	publish func(ctx context.Context, queue string, body []byte) error
}

func (f fakePublisher) Publish(ctx context.Context, queue string, body []byte) error {
	return f.publish(ctx, queue, body)
}

func TestPayoutPublisherDispatchesOutboxEvent(t *testing.T) {
	t.Parallel()

	expected := outbox.Event{
		ID:        "efb98fe4-b75f-4f1d-b9c7-794e66da2abb",
		EventType: outbox.EventTypeProcessPayout,
		EntityID:  "2c97a4da-38a7-46a8-9205-6482d0cfc6fb",
		Payload:   []byte(`{"payout_id":"abc","client_id":"def"}`),
	}

	published := false
	publisher := NewPayoutPublisher(fakePublisher{
		publish: func(_ context.Context, queue string, body []byte) error {
			published = true
			if queue != "payout.jobs" {
				t.Fatalf("expected queue payout.jobs, got %s", queue)
			}

			got, err := DecodePayoutJob(body)
			if err != nil {
				t.Fatalf("decode payout job: %v", err)
			}
			if got.ID != expected.ID {
				t.Fatalf("expected id %s, got %s", expected.ID, got.ID)
			}
			if got.EventType != expected.EventType {
				t.Fatalf("expected event type %s, got %s", expected.EventType, got.EventType)
			}
			if got.EntityID != expected.EntityID {
				t.Fatalf("expected entity id %s, got %s", expected.EntityID, got.EntityID)
			}
			if string(got.Payload) != string(expected.Payload) {
				t.Fatalf("expected payload %s, got %s", string(expected.Payload), string(got.Payload))
			}

			return nil
		},
	}, "payout.jobs")

	if err := publisher.Dispatch(context.Background(), expected); err != nil {
		t.Fatalf("dispatch payout event: %v", err)
	}
	if !published {
		t.Fatal("expected payout event to be published")
	}
}
