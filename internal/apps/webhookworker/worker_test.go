package webhookworker

import (
	"context"
	"errors"
	"io"
	"log"
	"testing"

	"github.com/prxgr4mm3r/payout-orchestrator/internal/outbox"
	platformrabbitmq "github.com/prxgr4mm3r/payout-orchestrator/internal/platform/rabbitmq"
)

type fakeDelivery struct {
	body     []byte
	ackCalls int
	nackArgs []bool
}

func (d *fakeDelivery) Body() []byte {
	return d.body
}

func (d *fakeDelivery) Ack() error {
	d.ackCalls++
	return nil
}

func (d *fakeDelivery) Nack(requeue bool) error {
	d.nackArgs = append(d.nackArgs, requeue)
	return nil
}

type fakeConsumer struct {
	consume func(ctx context.Context, queue string, handler func(context.Context, platformrabbitmq.Delivery) error) error
}

func (f fakeConsumer) Consume(ctx context.Context, queue string, handler func(context.Context, platformrabbitmq.Delivery) error) error {
	return f.consume(ctx, queue, handler)
}

type fakeHandler struct {
	handle func(ctx context.Context, event outbox.Event) error
}

func (f fakeHandler) HandleEvent(ctx context.Context, event outbox.Event) error {
	return f.handle(ctx, event)
}

func TestWebhookWorkerAckOnSuccess(t *testing.T) {
	t.Parallel()

	body := []byte(`{"id":"evt-1","event_type":"payout_result_webhook","entity_id":"payout-1","payload":"e30="}`)
	delivery := &fakeDelivery{body: body}
	handled := false

	worker := New(fakeConsumer{
		consume: func(ctx context.Context, queue string, handler func(context.Context, platformrabbitmq.Delivery) error) error {
			if queue != "webhook.deliveries" {
				t.Fatalf("expected queue webhook.deliveries, got %s", queue)
			}
			return handler(ctx, delivery)
		},
	}, fakeHandler{
		handle: func(_ context.Context, event outbox.Event) error {
			handled = true
			if event.ID != "evt-1" {
				t.Fatalf("expected event id evt-1, got %s", event.ID)
			}
			if event.EventType != outbox.EventTypePayoutResultWebhook {
				t.Fatalf("expected event type %s, got %s", outbox.EventTypePayoutResultWebhook, event.EventType)
			}
			return nil
		},
	}, "webhook.deliveries", log.New(io.Discard, "", 0))

	if err := worker.Run(context.Background()); err != nil {
		t.Fatalf("run webhook worker: %v", err)
	}
	if !handled {
		t.Fatal("expected webhook handler to be called")
	}
	if delivery.ackCalls != 1 {
		t.Fatalf("expected 1 ack call, got %d", delivery.ackCalls)
	}
	if len(delivery.nackArgs) != 0 {
		t.Fatalf("expected no nack calls, got %d", len(delivery.nackArgs))
	}
}

func TestWebhookWorkerNackRequeueOnHandlerError(t *testing.T) {
	t.Parallel()

	body := []byte(`{"id":"evt-1","event_type":"payout_result_webhook","entity_id":"payout-1","payload":"e30="}`)
	delivery := &fakeDelivery{body: body}
	expectedErr := errors.New("deliver webhook")

	worker := New(fakeConsumer{
		consume: func(ctx context.Context, _ string, handler func(context.Context, platformrabbitmq.Delivery) error) error {
			return handler(ctx, delivery)
		},
	}, fakeHandler{
		handle: func(context.Context, outbox.Event) error {
			return expectedErr
		},
	}, "webhook.deliveries", log.New(io.Discard, "", 0))

	if err := worker.Run(context.Background()); err != nil {
		t.Fatalf("run webhook worker: %v", err)
	}
	if delivery.ackCalls != 0 {
		t.Fatalf("expected 0 ack calls, got %d", delivery.ackCalls)
	}
	if len(delivery.nackArgs) != 1 || !delivery.nackArgs[0] {
		t.Fatalf("expected single nack with requeue=true, got %#v", delivery.nackArgs)
	}
}

func TestWebhookWorkerNackWithoutRequeueOnMalformedMessage(t *testing.T) {
	t.Parallel()

	delivery := &fakeDelivery{body: []byte("not-json")}

	worker := New(fakeConsumer{
		consume: func(ctx context.Context, _ string, handler func(context.Context, platformrabbitmq.Delivery) error) error {
			return handler(ctx, delivery)
		},
	}, fakeHandler{
		handle: func(context.Context, outbox.Event) error {
			t.Fatal("handler should not be called for malformed message")
			return nil
		},
	}, "webhook.deliveries", log.New(io.Discard, "", 0))

	if err := worker.Run(context.Background()); err != nil {
		t.Fatalf("run webhook worker: %v", err)
	}
	if delivery.ackCalls != 0 {
		t.Fatalf("expected 0 ack calls, got %d", delivery.ackCalls)
	}
	if len(delivery.nackArgs) != 1 || delivery.nackArgs[0] {
		t.Fatalf("expected single nack with requeue=false, got %#v", delivery.nackArgs)
	}
}
