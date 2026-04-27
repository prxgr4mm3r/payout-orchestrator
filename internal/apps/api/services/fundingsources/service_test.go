package fundingsources

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/prxgr4mm3r/payout-orchestrator/internal/db"
)

type fakeFundingSourceStore struct {
	create func(ctx context.Context, arg db.CreateFundingSourceParams) (db.FundingSource, error)
	get    func(ctx context.Context, arg db.GetFundingSourceByClientIDParams) (db.FundingSource, error)
	list   func(ctx context.Context, arg db.ListFundingSourcesByClientIDParams) ([]db.FundingSource, error)
}

func (f fakeFundingSourceStore) CreateFundingSource(ctx context.Context, arg db.CreateFundingSourceParams) (db.FundingSource, error) {
	return f.create(ctx, arg)
}

func (f fakeFundingSourceStore) GetFundingSourceByClientID(ctx context.Context, arg db.GetFundingSourceByClientIDParams) (db.FundingSource, error) {
	return f.get(ctx, arg)
}

func (f fakeFundingSourceStore) ListFundingSourcesByClientID(ctx context.Context, arg db.ListFundingSourcesByClientIDParams) ([]db.FundingSource, error) {
	return f.list(ctx, arg)
}

func TestCreateFundingSourcePersistsTrimmedInput(t *testing.T) {
	t.Parallel()

	clientID := mustUUID(t, "2c97a4da-38a7-46a8-9205-6482d0cfc6fb")
	sourceID := mustUUID(t, "efb98fe4-b75f-4f1d-b9c7-794e66da2abb")
	now := time.Date(2026, 4, 15, 12, 0, 0, 0, time.UTC)

	service := NewService(fakeFundingSourceStore{
		create: func(_ context.Context, arg db.CreateFundingSourceParams) (db.FundingSource, error) {
			if arg.ClientID != clientID {
				t.Fatalf("expected client id %s, got %s", clientID.String(), arg.ClientID.String())
			}
			if arg.Name != "Main account" {
				t.Fatalf("expected trimmed name, got %q", arg.Name)
			}
			if arg.Type != "bank_account" {
				t.Fatalf("expected trimmed type, got %q", arg.Type)
			}
			if arg.PaymentAccountID != "acct_123" {
				t.Fatalf("expected trimmed payment account id, got %q", arg.PaymentAccountID)
			}

			return dbFundingSource(sourceID, clientID, arg.Name, arg.PaymentAccountID, now), nil
		},
	})

	source, err := service.CreateFundingSource(context.Background(), CreateFundingSourceInput{
		ClientID:         clientID.String(),
		Name:             " Main account ",
		Type:             " bank_account ",
		PaymentAccountID: " acct_123 ",
	})
	if err != nil {
		t.Fatalf("create funding source: %v", err)
	}

	if source.ID != sourceID.String() {
		t.Fatalf("expected id %s, got %s", sourceID.String(), source.ID)
	}
	if source.ClientID != clientID.String() {
		t.Fatalf("expected client id %s, got %s", clientID.String(), source.ClientID)
	}
	if source.Status != "active" {
		t.Fatalf("expected status active, got %q", source.Status)
	}
	if !source.CreatedAt.Equal(now) {
		t.Fatalf("expected created at %s, got %s", now, source.CreatedAt)
	}
}

func TestCreateFundingSourceRejectsBlankFields(t *testing.T) {
	t.Parallel()

	service := NewService(fakeFundingSourceStore{
		create: func(context.Context, db.CreateFundingSourceParams) (db.FundingSource, error) {
			t.Fatal("store should not be called for invalid input")
			return db.FundingSource{}, nil
		},
	})

	_, err := service.CreateFundingSource(context.Background(), CreateFundingSourceInput{
		ClientID:         "2c97a4da-38a7-46a8-9205-6482d0cfc6fb",
		Name:             "Main account",
		Type:             "",
		PaymentAccountID: "acct_123",
	})
	if !errors.Is(err, ErrInvalidFundingSource) {
		t.Fatalf("expected ErrInvalidFundingSource, got %v", err)
	}
}

func TestCreateFundingSourceRejectsInvalidClientID(t *testing.T) {
	t.Parallel()

	service := NewService(fakeFundingSourceStore{
		create: func(context.Context, db.CreateFundingSourceParams) (db.FundingSource, error) {
			t.Fatal("store should not be called for invalid client id")
			return db.FundingSource{}, nil
		},
	})

	_, err := service.CreateFundingSource(context.Background(), CreateFundingSourceInput{
		ClientID:         "not-a-uuid",
		Name:             "Main account",
		Type:             "bank_account",
		PaymentAccountID: "acct_123",
	})
	if !errors.Is(err, ErrInvalidClientID) {
		t.Fatalf("expected ErrInvalidClientID, got %v", err)
	}
}

func TestGetFundingSourceLoadsClientScopedSource(t *testing.T) {
	t.Parallel()

	clientID := mustUUID(t, "2c97a4da-38a7-46a8-9205-6482d0cfc6fb")
	sourceID := mustUUID(t, "efb98fe4-b75f-4f1d-b9c7-794e66da2abb")
	now := time.Date(2026, 4, 15, 12, 0, 0, 0, time.UTC)

	service := NewService(fakeFundingSourceStore{
		get: func(_ context.Context, arg db.GetFundingSourceByClientIDParams) (db.FundingSource, error) {
			if arg.ClientID != clientID {
				t.Fatalf("expected client id %s, got %s", clientID.String(), arg.ClientID.String())
			}
			if arg.ID != sourceID {
				t.Fatalf("expected source id %s, got %s", sourceID.String(), arg.ID.String())
			}

			return dbFundingSource(sourceID, clientID, "Main account", "acct_123", now), nil
		},
	})

	source, err := service.GetFundingSource(context.Background(), GetFundingSourceInput{
		ClientID: clientID.String(),
		ID:       sourceID.String(),
	})
	if err != nil {
		t.Fatalf("get funding source: %v", err)
	}

	if source.ID != sourceID.String() {
		t.Fatalf("expected id %s, got %s", sourceID.String(), source.ID)
	}
	if source.ClientID != clientID.String() {
		t.Fatalf("expected client id %s, got %s", clientID.String(), source.ClientID)
	}
}

func TestGetFundingSourceMapsNotFound(t *testing.T) {
	t.Parallel()

	service := NewService(fakeFundingSourceStore{
		get: func(context.Context, db.GetFundingSourceByClientIDParams) (db.FundingSource, error) {
			return db.FundingSource{}, pgx.ErrNoRows
		},
	})

	_, err := service.GetFundingSource(context.Background(), GetFundingSourceInput{
		ClientID: "2c97a4da-38a7-46a8-9205-6482d0cfc6fb",
		ID:       "efb98fe4-b75f-4f1d-b9c7-794e66da2abb",
	})
	if !errors.Is(err, ErrFundingSourceNotFound) {
		t.Fatalf("expected ErrFundingSourceNotFound, got %v", err)
	}
}

func TestGetFundingSourceRejectsInvalidSourceID(t *testing.T) {
	t.Parallel()

	service := NewService(fakeFundingSourceStore{
		get: func(context.Context, db.GetFundingSourceByClientIDParams) (db.FundingSource, error) {
			t.Fatal("store should not be called for invalid source id")
			return db.FundingSource{}, nil
		},
	})

	_, err := service.GetFundingSource(context.Background(), GetFundingSourceInput{
		ClientID: "2c97a4da-38a7-46a8-9205-6482d0cfc6fb",
		ID:       "not-a-uuid",
	})
	if !errors.Is(err, ErrInvalidSourceID) {
		t.Fatalf("expected ErrInvalidSourceID, got %v", err)
	}
}

func TestListFundingSourcesLoadsClientScopedSources(t *testing.T) {
	t.Parallel()

	clientID := mustUUID(t, "2c97a4da-38a7-46a8-9205-6482d0cfc6fb")
	firstSourceID := mustUUID(t, "efb98fe4-b75f-4f1d-b9c7-794e66da2abb")
	secondSourceID := mustUUID(t, "b76e34c6-d2da-45b1-a0c1-307bc76918bd")
	now := time.Date(2026, 4, 15, 12, 0, 0, 0, time.UTC)

	service := NewService(fakeFundingSourceStore{
		list: func(_ context.Context, arg db.ListFundingSourcesByClientIDParams) ([]db.FundingSource, error) {
			if arg.ClientID != clientID {
				t.Fatalf("expected client id %s, got %s", clientID.String(), arg.ClientID.String())
			}
			if arg.Limit != 25 {
				t.Fatalf("expected limit 25, got %d", arg.Limit)
			}
			if arg.Offset != 10 {
				t.Fatalf("expected offset 10, got %d", arg.Offset)
			}

			return []db.FundingSource{
				dbFundingSource(firstSourceID, clientID, "Main account", "acct_123", now),
				dbFundingSource(secondSourceID, clientID, "Backup account", "acct_456", now),
			}, nil
		},
	})

	sources, err := service.ListFundingSources(context.Background(), ListFundingSourcesInput{
		ClientID: clientID.String(),
		Limit:    25,
		Offset:   10,
	})
	if err != nil {
		t.Fatalf("list funding sources: %v", err)
	}

	if len(sources) != 2 {
		t.Fatalf("expected 2 sources, got %d", len(sources))
	}
	if sources[0].ID != firstSourceID.String() {
		t.Fatalf("expected first id %s, got %s", firstSourceID.String(), sources[0].ID)
	}
	if sources[1].ID != secondSourceID.String() {
		t.Fatalf("expected second id %s, got %s", secondSourceID.String(), sources[1].ID)
	}
}

func TestListFundingSourcesRejectsInvalidPagination(t *testing.T) {
	t.Parallel()

	service := NewService(fakeFundingSourceStore{
		list: func(context.Context, db.ListFundingSourcesByClientIDParams) ([]db.FundingSource, error) {
			t.Fatal("store should not be called for invalid pagination")
			return nil, nil
		},
	})

	_, err := service.ListFundingSources(context.Background(), ListFundingSourcesInput{
		ClientID: "2c97a4da-38a7-46a8-9205-6482d0cfc6fb",
		Limit:    0,
		Offset:   0,
	})
	if !errors.Is(err, ErrInvalidPagination) {
		t.Fatalf("expected ErrInvalidPagination, got %v", err)
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

func dbFundingSource(id, clientID pgtype.UUID, name, paymentAccountID string, at time.Time) db.FundingSource {
	return db.FundingSource{
		ID:               id,
		ClientID:         clientID,
		Name:             name,
		Type:             "bank_account",
		PaymentAccountID: paymentAccountID,
		Status:           "active",
		CreatedAt:        pgtype.Timestamptz{Time: at, Valid: true},
		UpdatedAt:        pgtype.Timestamptz{Time: at, Valid: true},
	}
}
