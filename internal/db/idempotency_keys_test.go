package db

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
)

func TestIdempotencyKeyPayoutIDIsUnique(t *testing.T) {
	t.Parallel()

	dbURL := integrationTestDBURL()
	if dbURL == "" {
		t.Skip("set PAYOUT_SMOKE_DB_URL or DB_URL to run idempotency key persistence tests")
	}

	ctx := context.Background()
	adminPool := openPool(t, ctx, dbURL)
	t.Cleanup(adminPool.Close)

	schemaName := fmt.Sprintf("idempotency_key_%d", time.Now().UnixNano())
	if _, err := adminPool.Exec(ctx, "CREATE SCHEMA "+schemaName); err != nil {
		t.Fatalf("create schema: %v", err)
	}
	t.Cleanup(func() {
		if _, err := adminPool.Exec(context.Background(), "DROP SCHEMA IF EXISTS "+schemaName+" CASCADE"); err != nil {
			t.Fatalf("drop schema: %v", err)
		}
	})

	appPool := openPoolWithSearchPath(t, ctx, dbURL, schemaName)
	t.Cleanup(appPool.Close)

	applyMigrations(t, ctx, appPool)

	queries := New(appPool)
	client, err := queries.CreateClient(ctx, CreateClientParams{
		ID:     mustUUID(t, "2c97a4da-38a7-46a8-9205-6482d0cfc6fb"),
		Name:   "idempotency-client",
		ApiKey: mustUUID(t, "32d82e79-43e6-4e1f-8c29-71e5deba7e1d"),
	})
	if err != nil {
		t.Fatalf("create client: %v", err)
	}

	fundingSource, err := queries.CreateFundingSource(ctx, CreateFundingSourceParams{
		ClientID:         client.ID,
		Name:             "Primary account",
		Type:             "bank_account",
		PaymentAccountID: "acct_funding_123",
	})
	if err != nil {
		t.Fatalf("create funding source: %v", err)
	}

	payout, err := queries.CreatePayout(ctx, CreatePayoutParams{
		ClientID:           client.ID,
		FundingSourceID:    fundingSource.ID,
		ExternalID:         textValue("client-payout-1"),
		RecipientName:      textValue("Ada Lovelace"),
		RecipientAccountID: textValue("acct_recipient_123"),
		Amount:             numericValue(t, "125.50"),
		Currency:           "USDC",
	})
	if err != nil {
		t.Fatalf("create payout: %v", err)
	}

	if _, err := queries.CreateIdempotencyKey(ctx, CreateIdempotencyKeyParams{
		Key:         "first-key",
		ClientID:    client.ID,
		RequestHash: "first-request-hash",
		PayoutID:    payout.ID,
	}); err != nil {
		t.Fatalf("create first idempotency key: %v", err)
	}

	_, err = queries.CreateIdempotencyKey(ctx, CreateIdempotencyKeyParams{
		Key:         "second-key",
		ClientID:    client.ID,
		RequestHash: "second-request-hash",
		PayoutID:    payout.ID,
	})
	if err == nil {
		t.Fatal("expected duplicate payout id to be rejected")
	}

	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) {
		t.Fatalf("expected pg error, got %T: %v", err, err)
	}
	if pgErr.Code != "23505" {
		t.Fatalf("expected unique violation 23505, got %s", pgErr.Code)
	}
	if pgErr.ConstraintName != "idempotency_keys_payout_id_key" {
		t.Fatalf("expected constraint idempotency_keys_payout_id_key, got %s", pgErr.ConstraintName)
	}
}
