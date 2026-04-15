package payouts

import (
	"context"
	"errors"
	"math/big"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/prxgr4mm3r/payout-orchestrator/internal/db"
)

type fakePayoutStore struct {
	create           func(ctx context.Context, arg db.CreatePayoutParams) (db.Payout, error)
	get              func(ctx context.Context, arg db.GetPayoutByClientIDParams) (db.Payout, error)
	getFundingSource func(ctx context.Context, arg db.GetFundingSourceByClientIDParams) (db.FundingSource, error)
	list             func(ctx context.Context, arg db.ListPayoutsByClientIDParams) ([]db.Payout, error)
}

func (f fakePayoutStore) CreatePayout(ctx context.Context, arg db.CreatePayoutParams) (db.Payout, error) {
	return f.create(ctx, arg)
}

func (f fakePayoutStore) GetFundingSourceByClientID(ctx context.Context, arg db.GetFundingSourceByClientIDParams) (db.FundingSource, error) {
	return f.getFundingSource(ctx, arg)
}

func (f fakePayoutStore) GetPayoutByClientID(ctx context.Context, arg db.GetPayoutByClientIDParams) (db.Payout, error) {
	return f.get(ctx, arg)
}

func (f fakePayoutStore) ListPayoutsByClientID(ctx context.Context, arg db.ListPayoutsByClientIDParams) ([]db.Payout, error) {
	return f.list(ctx, arg)
}

func TestCreatePayoutValidatesFundingSourceOwnership(t *testing.T) {
	t.Parallel()

	clientID := mustUUID(t, "2c97a4da-38a7-46a8-9205-6482d0cfc6fb")
	payoutID := mustUUID(t, "efb98fe4-b75f-4f1d-b9c7-794e66da2abb")
	fundingSourceID := mustUUID(t, "b76e34c6-d2da-45b1-a0c1-307bc76918bd")
	now := time.Date(2026, 4, 15, 12, 0, 0, 0, time.UTC)

	service := NewService(fakePayoutStore{
		getFundingSource: func(_ context.Context, arg db.GetFundingSourceByClientIDParams) (db.FundingSource, error) {
			if arg.ClientID != clientID {
				t.Fatalf("expected client id %s, got %s", clientID.String(), arg.ClientID.String())
			}
			if arg.ID != fundingSourceID {
				t.Fatalf("expected funding source id %s, got %s", fundingSourceID.String(), arg.ID.String())
			}

			return db.FundingSource{ID: fundingSourceID, ClientID: clientID}, nil
		},
		create: func(_ context.Context, arg db.CreatePayoutParams) (db.Payout, error) {
			if arg.ClientID != clientID {
				t.Fatalf("expected client id %s, got %s", clientID.String(), arg.ClientID.String())
			}
			if arg.FundingSourceID != fundingSourceID {
				t.Fatalf("expected funding source id %s, got %s", fundingSourceID.String(), arg.FundingSourceID.String())
			}
			if got, err := numericString(arg.Amount); err != nil || got != "125.50" {
				t.Fatalf("expected amount 125.50, got %s: %v", got, err)
			}
			if arg.Currency != "USDC" {
				t.Fatalf("expected currency USDC, got %s", arg.Currency)
			}

			return dbPayout(payoutID, clientID, fundingSourceID, "125.50", "USDC", "pending", now), nil
		},
	})

	payout, err := service.CreatePayout(context.Background(), CreatePayoutInput{
		ClientID:        clientID.String(),
		FundingSourceID: fundingSourceID.String(),
		Amount:          "125.50",
		Currency:        " USDC ",
	})
	if err != nil {
		t.Fatalf("create payout: %v", err)
	}

	if payout.ID != payoutID.String() {
		t.Fatalf("expected id %s, got %s", payoutID.String(), payout.ID)
	}
	if payout.Status != "pending" {
		t.Fatalf("expected status pending, got %s", payout.Status)
	}
}

func TestCreatePayoutRejectsUnownedFundingSource(t *testing.T) {
	t.Parallel()

	service := NewService(fakePayoutStore{
		getFundingSource: func(context.Context, db.GetFundingSourceByClientIDParams) (db.FundingSource, error) {
			return db.FundingSource{}, pgx.ErrNoRows
		},
		create: func(context.Context, db.CreatePayoutParams) (db.Payout, error) {
			t.Fatal("store should not create payout for unowned funding source")
			return db.Payout{}, nil
		},
	})

	_, err := service.CreatePayout(context.Background(), CreatePayoutInput{
		ClientID:        "2c97a4da-38a7-46a8-9205-6482d0cfc6fb",
		FundingSourceID: "b76e34c6-d2da-45b1-a0c1-307bc76918bd",
		Amount:          "125.50",
		Currency:        "USDC",
	})
	if !errors.Is(err, ErrFundingSourceNotFound) {
		t.Fatalf("expected ErrFundingSourceNotFound, got %v", err)
	}
}

func TestCreatePayoutRejectsInvalidInput(t *testing.T) {
	t.Parallel()

	service := NewService(fakePayoutStore{
		getFundingSource: func(context.Context, db.GetFundingSourceByClientIDParams) (db.FundingSource, error) {
			t.Fatal("store should not be called for invalid payout")
			return db.FundingSource{}, nil
		},
	})

	_, err := service.CreatePayout(context.Background(), CreatePayoutInput{
		ClientID:        "2c97a4da-38a7-46a8-9205-6482d0cfc6fb",
		FundingSourceID: "b76e34c6-d2da-45b1-a0c1-307bc76918bd",
		Amount:          "0",
		Currency:        "USDC",
	})
	if !errors.Is(err, ErrInvalidPayout) {
		t.Fatalf("expected ErrInvalidPayout, got %v", err)
	}
}

func TestGetPayoutLoadsClientScopedPayout(t *testing.T) {
	t.Parallel()

	clientID := mustUUID(t, "2c97a4da-38a7-46a8-9205-6482d0cfc6fb")
	payoutID := mustUUID(t, "efb98fe4-b75f-4f1d-b9c7-794e66da2abb")
	fundingSourceID := mustUUID(t, "b76e34c6-d2da-45b1-a0c1-307bc76918bd")
	now := time.Date(2026, 4, 15, 12, 0, 0, 0, time.UTC)

	service := NewService(fakePayoutStore{
		get: func(_ context.Context, arg db.GetPayoutByClientIDParams) (db.Payout, error) {
			if arg.ClientID != clientID {
				t.Fatalf("expected client id %s, got %s", clientID.String(), arg.ClientID.String())
			}
			if arg.ID != payoutID {
				t.Fatalf("expected payout id %s, got %s", payoutID.String(), arg.ID.String())
			}

			return dbPayout(payoutID, clientID, fundingSourceID, "125.50", "USDC", "pending", now), nil
		},
	})

	payout, err := service.GetPayout(context.Background(), GetPayoutInput{
		ClientID: clientID.String(),
		ID:       payoutID.String(),
	})
	if err != nil {
		t.Fatalf("get payout: %v", err)
	}

	if payout.ID != payoutID.String() {
		t.Fatalf("expected id %s, got %s", payoutID.String(), payout.ID)
	}
	if payout.ClientID != clientID.String() {
		t.Fatalf("expected client id %s, got %s", clientID.String(), payout.ClientID)
	}
	if payout.FundingSourceID != fundingSourceID.String() {
		t.Fatalf("expected funding source id %s, got %s", fundingSourceID.String(), payout.FundingSourceID)
	}
	if payout.Amount != "125.50" {
		t.Fatalf("expected amount 125.50, got %s", payout.Amount)
	}
	if payout.Currency != "USDC" {
		t.Fatalf("expected currency USDC, got %s", payout.Currency)
	}
}

func TestGetPayoutMapsNotFound(t *testing.T) {
	t.Parallel()

	service := NewService(fakePayoutStore{
		get: func(context.Context, db.GetPayoutByClientIDParams) (db.Payout, error) {
			return db.Payout{}, pgx.ErrNoRows
		},
	})

	_, err := service.GetPayout(context.Background(), GetPayoutInput{
		ClientID: "2c97a4da-38a7-46a8-9205-6482d0cfc6fb",
		ID:       "efb98fe4-b75f-4f1d-b9c7-794e66da2abb",
	})
	if !errors.Is(err, ErrPayoutNotFound) {
		t.Fatalf("expected ErrPayoutNotFound, got %v", err)
	}
}

func TestGetPayoutRejectsInvalidPayoutID(t *testing.T) {
	t.Parallel()

	service := NewService(fakePayoutStore{
		get: func(context.Context, db.GetPayoutByClientIDParams) (db.Payout, error) {
			t.Fatal("store should not be called for invalid payout id")
			return db.Payout{}, nil
		},
	})

	_, err := service.GetPayout(context.Background(), GetPayoutInput{
		ClientID: "2c97a4da-38a7-46a8-9205-6482d0cfc6fb",
		ID:       "not-a-uuid",
	})
	if !errors.Is(err, ErrInvalidPayoutID) {
		t.Fatalf("expected ErrInvalidPayoutID, got %v", err)
	}
}

func TestListPayoutsLoadsClientScopedPayouts(t *testing.T) {
	t.Parallel()

	clientID := mustUUID(t, "2c97a4da-38a7-46a8-9205-6482d0cfc6fb")
	firstPayoutID := mustUUID(t, "efb98fe4-b75f-4f1d-b9c7-794e66da2abb")
	secondPayoutID := mustUUID(t, "5bc7aaf3-bb45-46ea-887f-e81b690e6730")
	fundingSourceID := mustUUID(t, "b76e34c6-d2da-45b1-a0c1-307bc76918bd")
	now := time.Date(2026, 4, 15, 12, 0, 0, 0, time.UTC)

	service := NewService(fakePayoutStore{
		list: func(_ context.Context, arg db.ListPayoutsByClientIDParams) ([]db.Payout, error) {
			if arg.ClientID != clientID {
				t.Fatalf("expected client id %s, got %s", clientID.String(), arg.ClientID.String())
			}
			if arg.Limit != 25 {
				t.Fatalf("expected limit 25, got %d", arg.Limit)
			}
			if arg.Offset != 10 {
				t.Fatalf("expected offset 10, got %d", arg.Offset)
			}

			return []db.Payout{
				dbPayout(firstPayoutID, clientID, fundingSourceID, "125.50", "USDC", "pending", now),
				dbPayout(secondPayoutID, clientID, fundingSourceID, "99.99", "USD", "processing", now),
			}, nil
		},
	})

	payouts, err := service.ListPayouts(context.Background(), ListPayoutsInput{
		ClientID: clientID.String(),
		Limit:    25,
		Offset:   10,
	})
	if err != nil {
		t.Fatalf("list payouts: %v", err)
	}

	if len(payouts) != 2 {
		t.Fatalf("expected 2 payouts, got %d", len(payouts))
	}
	if payouts[0].ID != firstPayoutID.String() {
		t.Fatalf("expected first id %s, got %s", firstPayoutID.String(), payouts[0].ID)
	}
	if payouts[1].Amount != "99.99" {
		t.Fatalf("expected second amount 99.99, got %s", payouts[1].Amount)
	}
}

func TestListPayoutsRejectsInvalidPagination(t *testing.T) {
	t.Parallel()

	service := NewService(fakePayoutStore{
		list: func(context.Context, db.ListPayoutsByClientIDParams) ([]db.Payout, error) {
			t.Fatal("store should not be called for invalid pagination")
			return nil, nil
		},
	})

	_, err := service.ListPayouts(context.Background(), ListPayoutsInput{
		ClientID: "2c97a4da-38a7-46a8-9205-6482d0cfc6fb",
		Limit:    0,
		Offset:   0,
	})
	if !errors.Is(err, ErrInvalidPagination) {
		t.Fatalf("expected ErrInvalidPagination, got %v", err)
	}
}

func TestNumericStringFormatsScales(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		in   pgtype.Numeric
		want string
	}{
		{name: "cents", in: numeric("12550", -2), want: "125.50"},
		{name: "leading zeros", in: numeric("99", -4), want: "0.0099"},
		{name: "whole", in: numeric("125", 0), want: "125"},
		{name: "positive exponent", in: numeric("125", 2), want: "12500"},
		{name: "negative", in: numeric("-12550", -2), want: "-125.50"},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := numericString(tt.in)
			if err != nil {
				t.Fatalf("numeric string: %v", err)
			}
			if got != tt.want {
				t.Fatalf("expected %s, got %s", tt.want, got)
			}
		})
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

func dbPayout(id, clientID, fundingSourceID pgtype.UUID, amount, currency, status string, at time.Time) db.Payout {
	return db.Payout{
		ID:              id,
		ClientID:        clientID,
		FundingSourceID: fundingSourceID,
		Amount:          numericFromDecimal(amount),
		Currency:        currency,
		Status:          status,
		CreatedAt:       pgtype.Timestamptz{Time: at, Valid: true},
		UpdatedAt:       pgtype.Timestamptz{Time: at, Valid: true},
	}
}

func numeric(rawInt string, exp int32) pgtype.Numeric {
	intValue, ok := new(big.Int).SetString(rawInt, 10)
	if !ok {
		panic("invalid test numeric")
	}

	return pgtype.Numeric{Int: intValue, Exp: exp, Valid: true}
}

func numericFromDecimal(raw string) pgtype.Numeric {
	parts := strings.Split(raw, ".")
	if len(parts) == 1 {
		return numeric(parts[0], 0)
	}

	return numeric(parts[0]+parts[1], int32(-len(parts[1])))
}
