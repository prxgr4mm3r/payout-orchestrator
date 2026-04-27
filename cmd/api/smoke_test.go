package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	amqp "github.com/rabbitmq/amqp091-go"

	"github.com/prxgr4mm3r/payout-orchestrator/internal/apps/api/handlers"
	"github.com/prxgr4mm3r/payout-orchestrator/internal/apps/api/middleware"
	authservice "github.com/prxgr4mm3r/payout-orchestrator/internal/apps/api/services/auth"
	fundingservice "github.com/prxgr4mm3r/payout-orchestrator/internal/apps/api/services/fundingsources"
	payoutservice "github.com/prxgr4mm3r/payout-orchestrator/internal/apps/api/services/payouts"
	payoutworker "github.com/prxgr4mm3r/payout-orchestrator/internal/apps/payoutworker"
	"github.com/prxgr4mm3r/payout-orchestrator/internal/apps/payoutworker/execution"
	rabbitmqbroker "github.com/prxgr4mm3r/payout-orchestrator/internal/broker/rabbitmq"
	"github.com/prxgr4mm3r/payout-orchestrator/internal/db"
	"github.com/prxgr4mm3r/payout-orchestrator/internal/outbox"
	platformrabbitmq "github.com/prxgr4mm3r/payout-orchestrator/internal/platform/rabbitmq"
	"github.com/prxgr4mm3r/payout-orchestrator/internal/providersimulator"
)

func TestMVPPayoutSmoke(t *testing.T) {
	t.Parallel()

	if testing.Short() {
		t.Skip("skipping smoke test in short mode")
	}

	dbURL := smokeTestDBURL()
	if dbURL == "" {
		t.Skip("set PAYOUT_SMOKE_DB_URL or DB_URL to run the smoke test")
	}

	ctx := context.Background()
	adminPool := openPool(t, ctx, dbURL)
	t.Cleanup(adminPool.Close)

	schemaName := fmt.Sprintf("mvp_smoke_%d", time.Now().UnixNano())
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

	queries := db.New(appPool)
	clientID := mustUUID(t, "2c97a4da-38a7-46a8-9205-6482d0cfc6fb")
	apiKey := mustUUID(t, "32d82e79-43e6-4e1f-8c29-71e5deba7e1d")

	clientRecord, err := queries.CreateClient(ctx, db.CreateClientParams{
		ID:     clientID,
		Name:   "smoke-client",
		ApiKey: apiKey,
	})
	if err != nil {
		t.Fatalf("create client: %v", err)
	}
	clientRecord, err = queries.UpdateClientWebhookURL(ctx, db.UpdateClientWebhookURLParams{
		ID:         clientRecord.ID,
		WebhookUrl: pgtype.Text{String: "https://example.com/webhooks/payouts", Valid: true},
	})
	if err != nil {
		t.Fatalf("update client webhook url: %v", err)
	}

	authSvc := authservice.NewService(queries)
	fundingSourcesSvc := fundingservice.NewService(queries)
	payoutsSvc := payoutservice.NewServiceWithTx(queries, payoutservice.NewDBTxRunner(appPool, queries))
	logger := log.New(io.Discard, "", 0)
	outboxRelay := outbox.NewRelay(
		outbox.NewDBTxRunner(appPool, queries),
		outbox.NewInlineDispatcher(execution.NewHandler(
			execution.NewDBTxRunner(appPool, queries),
			providersimulator.New(providersimulator.Config{}),
			logger,
		)),
		logger,
		outbox.Config{
			PollInterval: 10 * time.Millisecond,
			ClaimTimeout: time.Second,
		},
	)

	router := NewRouter(
		&handlers.ClientsHandler{},
		handlers.NewFundingSourcesHandler(fundingSourcesSvc),
		handlers.NewPayoutsHandler(payoutsSvc),
		middleware.APIKey(authSvc),
	)

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	t.Cleanup(func() {
		_ = listener.Close()
	})

	server := &http.Server{Handler: router}
	runCtx, cancelRun := context.WithCancel(context.Background())
	defer cancelRun()

	runErrCh := make(chan error, 1)
	go func() {
		runErrCh <- runApplication(
			runCtx,
			server,
			func() error { return server.Serve(listener) },
			outboxRelay.Run,
			5*time.Second,
			log.New(io.Discard, "", 0),
		)
	}()
	t.Cleanup(func() {
		cancelRun()
		if err := <-runErrCh; err != nil {
			t.Fatalf("stop application: %v", err)
		}
	})

	baseURL := "http://" + listener.Addr().String()
	waitForHealthz(t, baseURL)

	fundingSource := createFundingSourceOverHTTP(t, baseURL, clientRecord.ApiKey.String())
	payout := createPayoutOverHTTP(t, baseURL, clientRecord.ApiKey.String(), fundingSource.ID)
	finalPayout := waitForPayoutStatus(t, baseURL, clientRecord.ApiKey.String(), payout.ID, "succeeded")

	if finalPayout.Status != "succeeded" {
		t.Fatalf("expected final payout status succeeded, got %s", finalPayout.Status)
	}

	payoutRecord, err := queries.GetPayoutByClientID(ctx, db.GetPayoutByClientIDParams{
		ClientID: clientID,
		ID:       mustUUID(t, payout.ID),
	})
	if err != nil {
		t.Fatalf("load payout from db: %v", err)
	}
	if payoutRecord.Status != "succeeded" {
		t.Fatalf("expected db payout status succeeded, got %s", payoutRecord.Status)
	}

	waitForOutboxProcessed(t, ctx, appPool, mustUUID(t, payout.ID))

	assertPendingWebhookDelivery(t, ctx, queries, mustUUID(t, payout.ID), clientID, clientRecord.WebhookUrl.String, "succeeded")
}

func TestRabbitMQPayoutWorkerSmoke(t *testing.T) {
	t.Parallel()

	if testing.Short() {
		t.Skip("skipping smoke test in short mode")
	}

	dbURL := smokeTestDBURL()
	if dbURL == "" {
		t.Skip("set PAYOUT_SMOKE_DB_URL or DB_URL to run the smoke test")
	}
	rabbitmqURL := smokeTestRabbitMQURL()
	if rabbitmqURL == "" {
		t.Skip("set RABBITMQ_URL to run the RabbitMQ smoke test")
	}

	ctx := context.Background()
	adminPool := openPool(t, ctx, dbURL)
	t.Cleanup(adminPool.Close)

	schemaName := fmt.Sprintf("rabbitmq_smoke_%d", time.Now().UnixNano())
	if _, err := adminPool.Exec(ctx, "CREATE SCHEMA "+schemaName); err != nil {
		t.Fatalf("create schema: %v", err)
	}
	t.Cleanup(func() {
		if _, err := adminPool.Exec(context.Background(), "DROP SCHEMA IF EXISTS "+schemaName+" CASCADE"); err != nil {
			t.Fatalf("drop schema: %v", err)
		}
	})

	queueName := fmt.Sprintf("payout.jobs.smoke.%d", time.Now().UnixNano())

	appPool := openPoolWithSearchPath(t, ctx, dbURL, schemaName)
	t.Cleanup(appPool.Close)

	applyMigrations(t, ctx, appPool)

	publisherClient, err := platformrabbitmq.Open(rabbitmqURL)
	if err != nil {
		t.Fatalf("open rabbitmq publisher: %v", err)
	}
	t.Cleanup(func() {
		if err := publisherClient.Close(); err != nil {
			t.Fatalf("close rabbitmq publisher: %v", err)
		}
	})

	consumerClient, err := platformrabbitmq.Open(rabbitmqURL)
	if err != nil {
		t.Fatalf("open rabbitmq consumer: %v", err)
	}
	t.Cleanup(func() {
		if err := consumerClient.Close(); err != nil {
			t.Fatalf("close rabbitmq consumer: %v", err)
		}
	})

	payoutTopology := rabbitmqbroker.NewPayoutTopology(queueName)
	if err := publisherClient.EnsureTopology(payoutTopology); err != nil {
		t.Fatalf("ensure payout topology: %v", err)
	}
	t.Cleanup(func() {
		deleteRabbitMQQueue(t, rabbitmqURL, queueName)
	})

	queries := db.New(appPool)
	clientID := mustUUID(t, "2c97a4da-38a7-46a8-9205-6482d0cfc6fb")
	apiKey := mustUUID(t, "32d82e79-43e6-4e1f-8c29-71e5deba7e1d")

	clientRecord, err := queries.CreateClient(ctx, db.CreateClientParams{
		ID:     clientID,
		Name:   "rabbitmq-smoke-client",
		ApiKey: apiKey,
	})
	if err != nil {
		t.Fatalf("create client: %v", err)
	}
	clientRecord, err = queries.UpdateClientWebhookURL(ctx, db.UpdateClientWebhookURLParams{
		ID:         clientRecord.ID,
		WebhookUrl: pgtype.Text{String: "https://example.com/webhooks/payouts", Valid: true},
	})
	if err != nil {
		t.Fatalf("update client webhook url: %v", err)
	}

	logger := log.New(io.Discard, "", 0)
	worker := payoutworker.New(
		consumerClient,
		execution.NewHandler(
			execution.NewDBTxRunner(appPool, queries),
			providersimulator.New(providersimulator.Config{}),
			logger,
		),
		queueName,
		logger,
	)

	workerCtx, cancelWorker := context.WithCancel(context.Background())
	workerErrCh := make(chan error, 1)
	go func() {
		workerErrCh <- worker.Run(workerCtx)
	}()
	t.Cleanup(func() {
		cancelWorker()
		if err := <-workerErrCh; err != nil && !errors.Is(err, context.Canceled) {
			t.Fatalf("stop payout worker: %v", err)
		}
	})

	authSvc := authservice.NewService(queries)
	fundingSourcesSvc := fundingservice.NewService(queries)
	payoutsSvc := payoutservice.NewServiceWithTx(queries, payoutservice.NewDBTxRunner(appPool, queries))
	outboxRelay := outbox.NewRelay(
		outbox.NewDBTxRunner(appPool, queries),
		rabbitmqbroker.NewPayoutPublisher(publisherClient, payoutTopology.ExchangeName, payoutTopology.RoutingKey),
		logger,
		outbox.Config{
			PollInterval: 10 * time.Millisecond,
			ClaimTimeout: time.Second,
		},
	)

	router := NewRouter(
		&handlers.ClientsHandler{},
		handlers.NewFundingSourcesHandler(fundingSourcesSvc),
		handlers.NewPayoutsHandler(payoutsSvc),
		middleware.APIKey(authSvc),
	)

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	t.Cleanup(func() {
		_ = listener.Close()
	})

	server := &http.Server{Handler: router}
	runCtx, cancelRun := context.WithCancel(context.Background())
	runErrCh := make(chan error, 1)
	go func() {
		runErrCh <- runApplication(
			runCtx,
			server,
			func() error { return server.Serve(listener) },
			outboxRelay.Run,
			5*time.Second,
			logger,
		)
	}()
	t.Cleanup(func() {
		cancelRun()
		if err := <-runErrCh; err != nil {
			t.Fatalf("stop application: %v", err)
		}
	})

	baseURL := "http://" + listener.Addr().String()
	waitForHealthz(t, baseURL)

	fundingSource := createFundingSourceOverHTTP(t, baseURL, clientRecord.ApiKey.String())
	payout := createPayoutOverHTTP(t, baseURL, clientRecord.ApiKey.String(), fundingSource.ID)
	finalPayout := waitForPayoutStatus(t, baseURL, clientRecord.ApiKey.String(), payout.ID, "succeeded")

	if finalPayout.Status != "succeeded" {
		t.Fatalf("expected final payout status succeeded, got %s", finalPayout.Status)
	}

	waitForOutboxProcessed(t, ctx, appPool, mustUUID(t, payout.ID))

	assertPendingWebhookDelivery(t, ctx, queries, mustUUID(t, payout.ID), clientID, clientRecord.WebhookUrl.String, "succeeded")
}

type smokeFundingSourceResponse struct {
	ID string `json:"id"`
}

type smokePayoutResponse struct {
	ID     string `json:"id"`
	Status string `json:"status"`
}

func assertPendingWebhookDelivery(
	t *testing.T,
	ctx context.Context,
	queries *db.Queries,
	payoutID pgtype.UUID,
	clientID pgtype.UUID,
	targetURL string,
	status string,
) {
	t.Helper()

	deliveries, err := queries.ListWebhookDeliveriesByPayoutID(ctx, payoutID)
	if err != nil {
		t.Fatalf("list webhook deliveries: %v", err)
	}
	if len(deliveries) != 1 {
		t.Fatalf("expected 1 webhook delivery, got %d", len(deliveries))
	}

	delivery := deliveries[0]
	if delivery.ClientID != clientID {
		t.Fatalf("expected webhook client id %s, got %s", clientID.String(), delivery.ClientID.String())
	}
	if delivery.TargetUrl != targetURL {
		t.Fatalf("expected webhook target url %s, got %s", targetURL, delivery.TargetUrl)
	}
	if delivery.Status != "pending" {
		t.Fatalf("expected webhook delivery status pending, got %s", delivery.Status)
	}
	if delivery.AttemptCount != 0 {
		t.Fatalf("expected webhook delivery attempt count 0, got %d", delivery.AttemptCount)
	}

	var payload outbox.PayoutResultWebhookPayload
	if err := json.Unmarshal(delivery.Payload, &payload); err != nil {
		t.Fatalf("unmarshal webhook payload: %v", err)
	}
	if payload.EventType != outbox.EventTypePayoutResultWebhook {
		t.Fatalf("expected webhook event type %s, got %s", outbox.EventTypePayoutResultWebhook, payload.EventType)
	}
	if payload.PayoutID != payoutID.String() {
		t.Fatalf("expected webhook payload payout id %s, got %s", payoutID.String(), payload.PayoutID)
	}
	if payload.ClientID != clientID.String() {
		t.Fatalf("expected webhook payload client id %s, got %s", clientID.String(), payload.ClientID)
	}
	if payload.Status != status {
		t.Fatalf("expected webhook payload status %s, got %s", status, payload.Status)
	}
}

func waitForOutboxProcessed(t *testing.T, ctx context.Context, pool *pgxpool.Pool, entityID pgtype.UUID) {
	t.Helper()

	deadline := time.Now().Add(5 * time.Second)
	var lastStatus string
	var lastProcessed bool
	var lastErr error

	for time.Now().Before(deadline) {
		lastErr = pool.QueryRow(ctx, `
			SELECT status, processed_at IS NOT NULL
			FROM outbox_events
			WHERE entity_id = $1
		`, entityID).Scan(&lastStatus, &lastProcessed)
		if lastErr == nil && lastStatus == "processed" && lastProcessed {
			return
		}

		time.Sleep(20 * time.Millisecond)
	}

	if lastErr != nil {
		t.Fatalf("load outbox event: %v", lastErr)
	}
	if lastStatus != "processed" {
		t.Fatalf("expected outbox event status processed, got %s", lastStatus)
	}
	if !lastProcessed {
		t.Fatal("expected outbox event to have processed_at timestamp")
	}
}

func smokeTestDBURL() string {
	if value := strings.TrimSpace(os.Getenv("PAYOUT_SMOKE_DB_URL")); value != "" {
		return value
	}

	return strings.TrimSpace(os.Getenv("DB_URL"))
}

func smokeTestRabbitMQURL() string {
	return strings.TrimSpace(os.Getenv("RABBITMQ_URL"))
}

func deleteRabbitMQQueue(t *testing.T, url, queueName string) {
	t.Helper()

	conn, err := amqp.Dial(url)
	if err != nil {
		t.Fatalf("open rabbitmq cleanup connection: %v", err)
	}
	defer conn.Close()

	channel, err := conn.Channel()
	if err != nil {
		t.Fatalf("open rabbitmq cleanup channel: %v", err)
	}
	defer channel.Close()

	if _, err := channel.QueueDelete(queueName, false, false, false); err != nil {
		t.Fatalf("delete rabbitmq queue: %v", err)
	}
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

func waitForHealthz(t *testing.T, baseURL string) {
	t.Helper()

	client := &http.Client{Timeout: time.Second}
	deadline := time.Now().Add(5 * time.Second)

	for time.Now().Before(deadline) {
		resp, err := client.Get(baseURL + "/healthz")
		if err == nil {
			_ = resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return
			}
		}

		time.Sleep(20 * time.Millisecond)
	}

	t.Fatal("healthz did not become ready")
}

func createFundingSourceOverHTTP(t *testing.T, baseURL, apiKey string) smokeFundingSourceResponse {
	t.Helper()

	body := map[string]string{
		"name":               "Main account",
		"type":               "bank_account",
		"payment_account_id": "acct_smoke_123",
	}

	var response smokeFundingSourceResponse
	doJSONRequest(t, requestSpec{
		Method: http.MethodPost,
		URL:    baseURL + "/funding-sources",
		APIKey: apiKey,
		Body:   body,
		Status: http.StatusCreated,
	}, &response)

	if response.ID == "" {
		t.Fatal("expected funding source id")
	}

	return response
}

func createPayoutOverHTTP(t *testing.T, baseURL, apiKey, fundingSourceID string) smokePayoutResponse {
	t.Helper()

	body := map[string]string{
		"funding_source_id":    fundingSourceID,
		"external_id":          "smoke-ext-1",
		"recipient_name":       "Smoke Recipient",
		"recipient_account_id": "acct_recipient_smoke_123",
		"amount":               "125.50",
		"currency":             "USDC",
	}

	var response smokePayoutResponse
	doJSONRequest(t, requestSpec{
		Method:         http.MethodPost,
		URL:            baseURL + "/payouts",
		APIKey:         apiKey,
		IdempotencyKey: "smoke-payout-1",
		Body:           body,
		Status:         http.StatusCreated,
	}, &response)

	if response.ID == "" {
		t.Fatal("expected payout id")
	}

	return response
}

func waitForPayoutStatus(t *testing.T, baseURL, apiKey, payoutID, expectedStatus string) smokePayoutResponse {
	t.Helper()

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		var response smokePayoutResponse
		doJSONRequest(t, requestSpec{
			Method: http.MethodGet,
			URL:    baseURL + "/payouts/" + payoutID,
			APIKey: apiKey,
			Status: http.StatusOK,
		}, &response)
		if response.Status == expectedStatus {
			return response
		}

		time.Sleep(20 * time.Millisecond)
	}

	t.Fatalf("payout %s did not reach status %s", payoutID, expectedStatus)
	return smokePayoutResponse{}
}

type requestSpec struct {
	Method         string
	URL            string
	APIKey         string
	IdempotencyKey string
	Body           any
	Status         int
}

func doJSONRequest(t *testing.T, spec requestSpec, target any) {
	t.Helper()

	var bodyReader io.Reader
	if spec.Body != nil {
		payload, err := json.Marshal(spec.Body)
		if err != nil {
			t.Fatalf("marshal request body: %v", err)
		}
		bodyReader = bytes.NewReader(payload)
	}

	req, err := http.NewRequest(spec.Method, spec.URL, bodyReader)
	if err != nil {
		t.Fatalf("create request: %v", err)
	}
	if spec.Body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if spec.APIKey != "" {
		req.Header.Set("X-API-Key", spec.APIKey)
	}
	if spec.IdempotencyKey != "" {
		req.Header.Set("Idempotency-Key", spec.IdempotencyKey)
	}

	resp, err := (&http.Client{Timeout: 2 * time.Second}).Do(req)
	if err != nil {
		t.Fatalf("do request %s %s: %v", spec.Method, spec.URL, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != spec.Status {
		responseBody, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected %d from %s %s, got %d: %s", spec.Status, spec.Method, spec.URL, resp.StatusCode, strings.TrimSpace(string(responseBody)))
	}

	if target == nil {
		return
	}
	if err := json.NewDecoder(resp.Body).Decode(target); err != nil {
		t.Fatalf("decode response body: %v", err)
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
