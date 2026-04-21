package db

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestWebhookDeliveryPersistence(t *testing.T) {
	t.Parallel()

	dbURL := integrationTestDBURL()
	if dbURL == "" {
		t.Skip("set PAYOUT_SMOKE_DB_URL or DB_URL to run webhook delivery persistence tests")
	}

	ctx := context.Background()
	adminPool := openPool(t, ctx, dbURL)
	t.Cleanup(adminPool.Close)

	schemaName := fmt.Sprintf("webhook_delivery_%d", time.Now().UnixNano())
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
	clientID := mustUUID(t, "2c97a4da-38a7-46a8-9205-6482d0cfc6fb")
	apiKey := mustUUID(t, "32d82e79-43e6-4e1f-8c29-71e5deba7e1d")

	client, err := queries.CreateClient(ctx, CreateClientParams{
		ID:     clientID,
		Name:   "webhook-client",
		ApiKey: apiKey,
	})
	if err != nil {
		t.Fatalf("create client: %v", err)
	}

	client, err = queries.UpdateClientWebhookURL(ctx, UpdateClientWebhookURLParams{
		WebhookUrl: textValue("https://example.com/webhooks/payouts"),
		ID:         client.ID,
	})
	if err != nil {
		t.Fatalf("update client webhook url: %v", err)
	}
	if !client.WebhookUrl.Valid || client.WebhookUrl.String != "https://example.com/webhooks/payouts" {
		t.Fatalf("expected webhook url to be persisted, got %#v", client.WebhookUrl)
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

	delivery, err := queries.CreateWebhookDelivery(ctx, CreateWebhookDeliveryParams{
		PayoutID:     payout.ID,
		ClientID:     client.ID,
		TargetUrl:    client.WebhookUrl.String,
		Payload:      []byte(`{"event":"payout.updated","payout_id":"` + payout.ID.String() + `"}`),
		Status:       "pending",
		AttemptCount: 0,
	})
	if err != nil {
		t.Fatalf("create webhook delivery: %v", err)
	}

	if delivery.PayoutID != payout.ID {
		t.Fatalf("expected payout id %s, got %s", payout.ID.String(), delivery.PayoutID.String())
	}
	if delivery.ClientID != client.ID {
		t.Fatalf("expected client id %s, got %s", client.ID.String(), delivery.ClientID.String())
	}
	if delivery.TargetUrl != client.WebhookUrl.String {
		t.Fatalf("expected target url %s, got %s", client.WebhookUrl.String, delivery.TargetUrl)
	}
	if delivery.Status != "pending" {
		t.Fatalf("expected status pending, got %s", delivery.Status)
	}
	if delivery.AttemptCount != 0 {
		t.Fatalf("expected attempt count 0, got %d", delivery.AttemptCount)
	}

	storedDelivery, err := queries.GetWebhookDelivery(ctx, delivery.ID)
	if err != nil {
		t.Fatalf("get webhook delivery: %v", err)
	}
	if string(storedDelivery.Payload) != string(delivery.Payload) {
		t.Fatalf("expected payload %s, got %s", string(delivery.Payload), string(storedDelivery.Payload))
	}

	deliveries, err := queries.ListWebhookDeliveriesByPayoutID(ctx, payout.ID)
	if err != nil {
		t.Fatalf("list webhook deliveries: %v", err)
	}
	if len(deliveries) != 1 {
		t.Fatalf("expected 1 webhook delivery, got %d", len(deliveries))
	}
	if deliveries[0].ID != delivery.ID {
		t.Fatalf("expected listed delivery id %s, got %s", delivery.ID.String(), deliveries[0].ID.String())
	}
}

func integrationTestDBURL() string {
	if value := strings.TrimSpace(os.Getenv("PAYOUT_SMOKE_DB_URL")); value != "" {
		return value
	}

	return strings.TrimSpace(os.Getenv("DB_URL"))
}

func openPool(t *testing.T, ctx context.Context, dbURL string) *pgxpool.Pool {
	t.Helper()

	pool, err := pgxpool.New(ctx, dbURL)
	if err != nil {
		t.Fatalf("create pool: %v", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		t.Fatalf("ping pool: %v", err)
	}

	return pool
}

func openPoolWithSearchPath(t *testing.T, ctx context.Context, dbURL, searchPath string) *pgxpool.Pool {
	t.Helper()

	cfg, err := pgxpool.ParseConfig(dbURL)
	if err != nil {
		t.Fatalf("parse db config: %v", err)
	}
	cfg.ConnConfig.RuntimeParams["search_path"] = searchPath + ",public"

	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatalf("create scoped pool: %v", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		t.Fatalf("ping scoped pool: %v", err)
	}

	return pool
}

func applyMigrations(t *testing.T, ctx context.Context, pool *pgxpool.Pool) {
	t.Helper()

	pattern := filepath.Join(repoRoot(t), "migrations", "*.up.sql")
	files, err := filepath.Glob(pattern)
	if err != nil {
		t.Fatalf("glob migrations: %v", err)
	}
	sort.Strings(files)
	if len(files) == 0 {
		t.Fatal("no migration files found")
	}

	for _, file := range files {
		contents, err := os.ReadFile(file)
		if err != nil {
			t.Fatalf("read migration %s: %v", file, err)
		}

		for _, statement := range splitSQLStatements(string(contents)) {
			if _, err := pool.Exec(ctx, statement); err != nil {
				t.Fatalf("apply migration %s: %v", filepath.Base(file), err)
			}
		}
	}
}

func repoRoot(t *testing.T) string {
	t.Helper()

	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve current file path")
	}

	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", ".."))
}

func splitSQLStatements(contents string) []string {
	parts := strings.Split(contents, ";")
	statements := make([]string, 0, len(parts))
	for _, part := range parts {
		statement := strings.TrimSpace(part)
		if statement == "" {
			continue
		}

		statements = append(statements, statement)
	}

	return statements
}

func mustUUID(t *testing.T, raw string) pgtype.UUID {
	t.Helper()

	var id pgtype.UUID
	if err := id.Scan(raw); err != nil {
		t.Fatalf("scan uuid %q: %v", raw, err)
	}

	return id
}

func numericValue(t *testing.T, raw string) pgtype.Numeric {
	t.Helper()

	var value pgtype.Numeric
	if err := value.Scan(raw); err != nil {
		t.Fatalf("scan numeric %q: %v", raw, err)
	}

	return value
}

func textValue(value string) pgtype.Text {
	return pgtype.Text{String: value, Valid: value != ""}
}
