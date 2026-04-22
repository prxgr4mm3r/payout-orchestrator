package outbox

import (
	"context"
	"errors"
	"io"
	"log"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/prxgr4mm3r/payout-orchestrator/internal/db"
)

type fakeStore struct {
	claim         func(ctx context.Context, reclaimBefore pgtype.Timestamptz) (db.OutboxEvent, error)
	markProcessed func(ctx context.Context, id pgtype.UUID) (db.OutboxEvent, error)
	release       func(ctx context.Context, id pgtype.UUID) (db.OutboxEvent, error)
}

type fakeTxRunner struct {
	run func(ctx context.Context, fn func(store Store) error) error
}

func (f fakeTxRunner) WithinTx(ctx context.Context, fn func(store Store) error) error {
	return f.run(ctx, fn)
}

func (f fakeStore) ClaimNextPendingOutboxEvent(ctx context.Context, reclaimBefore pgtype.Timestamptz) (db.OutboxEvent, error) {
	return f.claim(ctx, reclaimBefore)
}

func (f fakeStore) MarkOutboxEventAsProcessed(ctx context.Context, id pgtype.UUID) (db.OutboxEvent, error) {
	return f.markProcessed(ctx, id)
}

func (f fakeStore) ReleaseOutboxEventClaim(ctx context.Context, id pgtype.UUID) (db.OutboxEvent, error) {
	return f.release(ctx, id)
}

func TestRunOncePublishesClaimedEvent(t *testing.T) {
	t.Parallel()

	eventID := mustUUID(t, "efb98fe4-b75f-4f1d-b9c7-794e66da2abb")
	entityID := mustUUID(t, "2c97a4da-38a7-46a8-9205-6482d0cfc6fb")
	published := false
	markedProcessed := false
	txCalls := 0

	publisher := NewPublisher(fakeTxRunner{
		run: func(ctx context.Context, fn func(store Store) error) error {
			txCalls++
			switch txCalls {
			case 1:
				return fn(fakeStore{
					claim: func(_ context.Context, reclaimBefore pgtype.Timestamptz) (db.OutboxEvent, error) {
						if !reclaimBefore.Valid || reclaimBefore.Time.IsZero() {
							t.Fatal("expected non-zero reclaim deadline")
						}

						return db.OutboxEvent{
							ID:        eventID,
							EntityID:  entityID,
							EventType: EventTypeProcessPayout,
							Payload:   []byte(`{"ok":true}`),
							Status:    "processing",
						}, nil
					},
					markProcessed: func(context.Context, pgtype.UUID) (db.OutboxEvent, error) {
						t.Fatal("mark processed should happen in a separate transaction")
						return db.OutboxEvent{}, nil
					},
					release: func(context.Context, pgtype.UUID) (db.OutboxEvent, error) {
						t.Fatal("release should not be called on successful publish")
						return db.OutboxEvent{}, nil
					},
				})
			case 2:
				return fn(fakeStore{
					claim: func(context.Context, pgtype.Timestamptz) (db.OutboxEvent, error) {
						t.Fatal("claim should not be called while marking processed")
						return db.OutboxEvent{}, nil
					},
					markProcessed: func(_ context.Context, id pgtype.UUID) (db.OutboxEvent, error) {
						if id != eventID {
							t.Fatalf("expected processed event id %s, got %s", eventID.String(), id.String())
						}
						markedProcessed = true
						return db.OutboxEvent{ID: id, Status: "processed"}, nil
					},
					release: func(context.Context, pgtype.UUID) (db.OutboxEvent, error) {
						t.Fatal("release should not be called on successful publish")
						return db.OutboxEvent{}, nil
					},
				})
			default:
				t.Fatalf("unexpected transaction call %d", txCalls)
				return nil
			}
		},
	}, EventPublisherFunc(func(_ context.Context, event PublishableEvent) error {
		published = true
		if event.ID != eventID.String() {
			t.Fatalf("expected event id %s, got %s", eventID.String(), event.ID)
		}
		if event.EntityID != entityID.String() {
			t.Fatalf("expected entity id %s, got %s", entityID.String(), event.EntityID)
		}
		if event.EventType != EventTypeProcessPayout {
			t.Fatalf("expected event type %s, got %s", EventTypeProcessPayout, event.EventType)
		}

		return nil
	}), log.New(io.Discard, "", 0), Config{ClaimTimeout: 15 * time.Second})

	claimed, err := publisher.RunOnce(context.Background())
	if err != nil {
		t.Fatalf("run once: %v", err)
	}
	if !claimed {
		t.Fatal("expected outbox event to be published")
	}
	if !published {
		t.Fatal("expected event publisher to be called")
	}
	if !markedProcessed {
		t.Fatal("expected outbox event to be marked processed")
	}
	if txCalls != 2 {
		t.Fatalf("expected 2 transactions, got %d", txCalls)
	}
}

func TestRunOnceReleasesClaimWhenPublishFails(t *testing.T) {
	t.Parallel()

	eventID := mustUUID(t, "efb98fe4-b75f-4f1d-b9c7-794e66da2abb")
	expectedErr := errors.New("publish event")
	released := false
	txCalls := 0

	publisher := NewPublisher(fakeTxRunner{
		run: func(ctx context.Context, fn func(store Store) error) error {
			txCalls++
			switch txCalls {
			case 1:
				return fn(fakeStore{
					claim: func(context.Context, pgtype.Timestamptz) (db.OutboxEvent, error) {
						return db.OutboxEvent{ID: eventID}, nil
					},
					markProcessed: func(context.Context, pgtype.UUID) (db.OutboxEvent, error) {
						t.Fatal("mark processed should not be called when publish fails")
						return db.OutboxEvent{}, nil
					},
					release: func(context.Context, pgtype.UUID) (db.OutboxEvent, error) {
						t.Fatal("release should happen in a separate transaction")
						return db.OutboxEvent{}, nil
					},
				})
			case 2:
				return fn(fakeStore{
					claim: func(context.Context, pgtype.Timestamptz) (db.OutboxEvent, error) {
						t.Fatal("claim should not be retried while releasing")
						return db.OutboxEvent{}, nil
					},
					markProcessed: func(context.Context, pgtype.UUID) (db.OutboxEvent, error) {
						t.Fatal("mark processed should not be called when publish fails")
						return db.OutboxEvent{}, nil
					},
					release: func(_ context.Context, id pgtype.UUID) (db.OutboxEvent, error) {
						if id != eventID {
							t.Fatalf("expected release id %s, got %s", eventID.String(), id.String())
						}
						released = true
						return db.OutboxEvent{ID: id, Status: "pending"}, nil
					},
				})
			default:
				t.Fatalf("unexpected transaction call %d", txCalls)
				return nil
			}
		},
	}, EventPublisherFunc(func(context.Context, PublishableEvent) error {
		return expectedErr
	}), log.New(io.Discard, "", 0), Config{})

	claimed, err := publisher.RunOnce(context.Background())
	if !errors.Is(err, expectedErr) {
		t.Fatalf("expected error %v, got %v", expectedErr, err)
	}
	if claimed {
		t.Fatal("expected failed publish not to report success")
	}
	if !released {
		t.Fatal("expected claim to be released")
	}
}

func TestInlinePublisherForwardsPublishedEvent(t *testing.T) {
	t.Parallel()

	expected := PublishableEvent{
		ID:        "efb98fe4-b75f-4f1d-b9c7-794e66da2abb",
		EventType: EventTypeProcessPayout,
		EntityID:  "2c97a4da-38a7-46a8-9205-6482d0cfc6fb",
		Payload:   []byte(`{"ok":true}`),
	}

	called := false
	publisher := NewInlinePublisher(publishedEventHandlerFunc(func(_ context.Context, event PublishableEvent) error {
		called = true
		if event.ID != expected.ID {
			t.Fatalf("expected id %s, got %s", expected.ID, event.ID)
		}
		if event.EventType != expected.EventType {
			t.Fatalf("expected event type %s, got %s", expected.EventType, event.EventType)
		}
		return nil
	}))

	if err := publisher.Publish(context.Background(), expected); err != nil {
		t.Fatalf("publish inline event: %v", err)
	}
	if !called {
		t.Fatal("expected inline publisher to call handler")
	}
}

type publishedEventHandlerFunc func(ctx context.Context, event PublishableEvent) error

func (f publishedEventHandlerFunc) HandlePublishedEvent(ctx context.Context, event PublishableEvent) error {
	return f(ctx, event)
}

func mustUUID(t *testing.T, raw string) pgtype.UUID {
	t.Helper()

	var id pgtype.UUID
	if err := id.Scan(raw); err != nil {
		t.Fatalf("scan uuid %q: %v", raw, err)
	}

	return id
}
