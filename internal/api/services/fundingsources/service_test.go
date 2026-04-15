package fundingsources

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/prxgr4mm3r/payout-orchestrator/internal/db"
)

type fakeFundingSourceStore struct {
	create func(ctx context.Context, arg db.CreateFundingSourceParams) (db.FundingSource, error)
}

func (f fakeFundingSourceStore) CreateFundingSource(ctx context.Context, arg db.CreateFundingSourceParams) (db.FundingSource, error) {
	return f.create(ctx, arg)
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

			return db.FundingSource{
				ID:               sourceID,
				ClientID:         clientID,
				Name:             arg.Name,
				Type:             arg.Type,
				PaymentAccountID: arg.PaymentAccountID,
				Status:           "active",
				CreatedAt:        pgtype.Timestamptz{Time: now, Valid: true},
				UpdatedAt:        pgtype.Timestamptz{Time: now, Valid: true},
			}, nil
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

func mustUUID(t *testing.T, raw string) pgtype.UUID {
	t.Helper()

	var id pgtype.UUID
	if err := id.Scan(raw); err != nil {
		t.Fatalf("scan uuid %q: %v", raw, err)
	}

	return id
}
