package rabbitmq

import (
	"context"
	"testing"

	"github.com/prxgr4mm3r/payout-orchestrator/internal/outbox"
)

func TestWebhookPublisherDispatchesOutboxEvent(t *testing.T) {
	t.Parallel()

	expected := outbox.Event{
		ID:        "efb98fe4-b75f-4f1d-b9c7-794e66da2abb",
		EventType: outbox.EventTypePayoutResultWebhook,
		EntityID:  "2c97a4da-38a7-46a8-9205-6482d0cfc6fb",
		Payload:   []byte(`{"event_type":"payout_result_webhook"}`),
	}

	published := false
	publisher := NewWebhookPublisher(fakePublisher{
		publish: func(_ context.Context, exchange, routingKey string, body []byte) error {
			published = true
			if exchange != WebhookExchangeName {
				t.Fatalf("expected exchange %s, got %s", WebhookExchangeName, exchange)
			}
			if routingKey != WebhookRoutingKey {
				t.Fatalf("expected routing key %s, got %s", WebhookRoutingKey, routingKey)
			}

			got, err := DecodeWebhookDeliveryJob(body)
			if err != nil {
				t.Fatalf("decode webhook delivery job: %v", err)
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
	}, WebhookExchangeName, WebhookRoutingKey)

	if err := publisher.Dispatch(context.Background(), expected); err != nil {
		t.Fatalf("dispatch webhook event: %v", err)
	}
	if !published {
		t.Fatal("expected webhook event to be published")
	}
}

func TestNewWebhookTopologyUsesConfiguredQueue(t *testing.T) {
	t.Parallel()

	topology := NewWebhookTopology("webhook.deliveries.test")

	if topology.ExchangeName != WebhookExchangeName {
		t.Fatalf("expected exchange %s, got %s", WebhookExchangeName, topology.ExchangeName)
	}
	if topology.QueueName != "webhook.deliveries.test" {
		t.Fatalf("expected queue webhook.deliveries.test, got %s", topology.QueueName)
	}
	if topology.RoutingKey != WebhookRoutingKey {
		t.Fatalf("expected routing key %s, got %s", WebhookRoutingKey, topology.RoutingKey)
	}
}
