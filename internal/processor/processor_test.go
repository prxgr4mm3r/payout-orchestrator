package processor

import (
	"context"
	"errors"
	"io"
	"log"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/prxgr4mm3r/payout-orchestrator/internal/db"
)

type fakeStore struct {
	claim   func(ctx context.Context, reclaimBefore pgtype.Timestamptz) (db.OutboxEvent, error)
	release func(ctx context.Context, id pgtype.UUID) (db.OutboxEvent, error)
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

func (f fakeStore) ReleaseOutboxEventClaim(ctx context.Context, id pgtype.UUID) (db.OutboxEvent, error) {
	return f.release(ctx, id)
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
	}, HandlerFunc(func(context.Context, db.OutboxEvent) error {
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
	}, HandlerFunc(func(context.Context, db.OutboxEvent) error {
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
	}, HandlerFunc(func(context.Context, db.OutboxEvent) error {
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
	}, HandlerFunc(func(context.Context, db.OutboxEvent) error {
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

func mustUUID(t *testing.T, raw string) pgtype.UUID {
	t.Helper()

	var id pgtype.UUID
	if err := id.Scan(raw); err != nil {
		t.Fatalf("scan uuid %q: %v", raw, err)
	}

	return id
}
