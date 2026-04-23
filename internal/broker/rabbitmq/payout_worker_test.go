package rabbitmq

import (
	"context"
	"errors"
	"io"
	"log"
	"testing"

	"github.com/prxgr4mm3r/payout-orchestrator/internal/outbox"
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
	consume func(ctx context.Context, queue string, handler func(context.Context, Delivery) error) error
}

func (f fakeConsumer) Consume(ctx context.Context, queue string, handler func(context.Context, Delivery) error) error {
	return f.consume(ctx, queue, handler)
}

type fakeHandler struct {
	handle func(ctx context.Context, event outbox.Event) error
}

func (f fakeHandler) HandleEvent(ctx context.Context, event outbox.Event) error {
	return f.handle(ctx, event)
}

func TestPayoutWorkerAckOnSuccess(t *testing.T) {
	t.Parallel()

	body := []byte(`{"id":"evt-1","event_type":"process_payout","entity_id":"payout-1","payload":"e30="}`)
	delivery := &fakeDelivery{body: body}
	handled := false

	worker := NewPayoutWorker(fakeConsumer{
		consume: func(ctx context.Context, queue string, handler func(context.Context, Delivery) error) error {
			if queue != "payout.jobs" {
				t.Fatalf("expected queue payout.jobs, got %s", queue)
			}
			return handler(ctx, delivery)
		},
	}, fakeHandler{
		handle: func(_ context.Context, event outbox.Event) error {
			handled = true
			if event.ID != "evt-1" {
				t.Fatalf("expected event id evt-1, got %s", event.ID)
			}
			if event.EventType != outbox.EventTypeProcessPayout {
				t.Fatalf("expected event type %s, got %s", outbox.EventTypeProcessPayout, event.EventType)
			}
			return nil
		},
	}, "payout.jobs", log.New(io.Discard, "", 0))

	if err := worker.Run(context.Background()); err != nil {
		t.Fatalf("run payout worker: %v", err)
	}
	if !handled {
		t.Fatal("expected payout handler to be called")
	}
	if delivery.ackCalls != 1 {
		t.Fatalf("expected 1 ack call, got %d", delivery.ackCalls)
	}
	if len(delivery.nackArgs) != 0 {
		t.Fatalf("expected no nack calls, got %d", len(delivery.nackArgs))
	}
}

func TestPayoutWorkerNackRequeueOnHandlerError(t *testing.T) {
	t.Parallel()

	body := []byte(`{"id":"evt-1","event_type":"process_payout","entity_id":"payout-1","payload":"e30="}`)
	delivery := &fakeDelivery{body: body}
	expectedErr := errors.New("execute payout")

	worker := NewPayoutWorker(fakeConsumer{
		consume: func(ctx context.Context, _ string, handler func(context.Context, Delivery) error) error {
			return handler(ctx, delivery)
		},
	}, fakeHandler{
		handle: func(context.Context, outbox.Event) error {
			return expectedErr
		},
	}, "payout.jobs", log.New(io.Discard, "", 0))

	if err := worker.Run(context.Background()); err != nil {
		t.Fatalf("run payout worker: %v", err)
	}
	if delivery.ackCalls != 0 {
		t.Fatalf("expected 0 ack calls, got %d", delivery.ackCalls)
	}
	if len(delivery.nackArgs) != 1 || !delivery.nackArgs[0] {
		t.Fatalf("expected single nack with requeue=true, got %#v", delivery.nackArgs)
	}
}

func TestPayoutWorkerNackWithoutRequeueOnMalformedMessage(t *testing.T) {
	t.Parallel()

	delivery := &fakeDelivery{body: []byte("not-json")}

	worker := NewPayoutWorker(fakeConsumer{
		consume: func(ctx context.Context, _ string, handler func(context.Context, Delivery) error) error {
			return handler(ctx, delivery)
		},
	}, fakeHandler{
		handle: func(context.Context, outbox.Event) error {
			t.Fatal("handler should not be called for malformed message")
			return nil
		},
	}, "payout.jobs", log.New(io.Discard, "", 0))

	if err := worker.Run(context.Background()); err != nil {
		t.Fatalf("run payout worker: %v", err)
	}
	if delivery.ackCalls != 0 {
		t.Fatalf("expected 0 ack calls, got %d", delivery.ackCalls)
	}
	if len(delivery.nackArgs) != 1 || delivery.nackArgs[0] {
		t.Fatalf("expected single nack with requeue=false, got %#v", delivery.nackArgs)
	}
}
