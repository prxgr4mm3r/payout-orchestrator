package rabbitmq

import (
	"context"
	"testing"

	"github.com/prxgr4mm3r/payout-orchestrator/internal/outbox"
)

type fakePublisher struct {
	publish func(ctx context.Context, exchange, routingKey string, body []byte) error
}

func (f fakePublisher) Publish(ctx context.Context, exchange, routingKey string, body []byte) error {
	return f.publish(ctx, exchange, routingKey, body)
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
		publish: func(_ context.Context, exchange, routingKey string, body []byte) error {
			published = true
			if exchange != PayoutExchangeName {
				t.Fatalf("expected exchange %s, got %s", PayoutExchangeName, exchange)
			}
			if routingKey != PayoutRoutingKey {
				t.Fatalf("expected routing key %s, got %s", PayoutRoutingKey, routingKey)
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
	}, PayoutExchangeName, PayoutRoutingKey)

	if err := publisher.Dispatch(context.Background(), expected); err != nil {
		t.Fatalf("dispatch payout event: %v", err)
	}
	if !published {
		t.Fatal("expected payout event to be published")
	}
}

func TestNewPayoutTopologyUsesConfiguredQueue(t *testing.T) {
	t.Parallel()

	topology := NewPayoutTopology("payout.jobs.test")

	if topology.ExchangeName != PayoutExchangeName {
		t.Fatalf("expected exchange %s, got %s", PayoutExchangeName, topology.ExchangeName)
	}
	if topology.QueueName != "payout.jobs.test" {
		t.Fatalf("expected queue payout.jobs.test, got %s", topology.QueueName)
	}
	if topology.RoutingKey != PayoutRoutingKey {
		t.Fatalf("expected routing key %s, got %s", PayoutRoutingKey, topology.RoutingKey)
	}
}
