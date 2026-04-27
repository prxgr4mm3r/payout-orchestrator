package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/prxgr4mm3r/payout-orchestrator/internal/apps/api/handlers"
	"github.com/prxgr4mm3r/payout-orchestrator/internal/apps/api/middleware"
	authservice "github.com/prxgr4mm3r/payout-orchestrator/internal/apps/api/services/auth"
	fundingservice "github.com/prxgr4mm3r/payout-orchestrator/internal/apps/api/services/fundingsources"
	payoutservice "github.com/prxgr4mm3r/payout-orchestrator/internal/apps/api/services/payouts"
	rabbitmqbroker "github.com/prxgr4mm3r/payout-orchestrator/internal/broker/rabbitmq"
	"github.com/prxgr4mm3r/payout-orchestrator/internal/db"
	"github.com/prxgr4mm3r/payout-orchestrator/internal/outbox"
	"github.com/prxgr4mm3r/payout-orchestrator/internal/platform/postgres"
	platformrabbitmq "github.com/prxgr4mm3r/payout-orchestrator/internal/platform/rabbitmq"
)

func main() {
	dbURL := os.Getenv("DB_URL")
	rabbitmqURL := os.Getenv("RABBITMQ_URL")
	payoutQueueName := loadStringEnv("PAYOUT_QUEUE_NAME", "payout.jobs")
	webhookQueueName := loadStringEnv("WEBHOOK_QUEUE_NAME", "webhook.deliveries")
	outboxRelayEnabled, err := loadBoolEnv("PROCESSOR_ENABLED", false)
	if err != nil {
		log.Fatalf("load PROCESSOR_ENABLED: %v", err)
	}
	pollInterval, err := loadDurationEnv("PROCESSOR_POLL_INTERVAL", time.Second)
	if err != nil {
		log.Fatalf("load PROCESSOR_POLL_INTERVAL: %v", err)
	}
	claimTimeout, err := loadDurationEnv("PROCESSOR_CLAIM_TIMEOUT", 30*time.Second)
	if err != nil {
		log.Fatalf("load PROCESSOR_CLAIM_TIMEOUT: %v", err)
	}

	startupCtx, startupCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer startupCancel()

	dbPool, err := postgres.Open(startupCtx, dbURL)
	if err != nil {
		log.Fatalf("open postgres: %v", err)
	}
	defer dbPool.Close()

	log.Println("postgres connection is ready")

	queries := db.New(dbPool)
	authSvc := authservice.NewService(queries)
	fundingSourcesSvc := fundingservice.NewService(queries)
	payoutsSvc := payoutservice.NewServiceWithTx(queries, payoutservice.NewDBTxRunner(dbPool, queries))
	clientsHandler := &handlers.ClientsHandler{}
	fundingSourcesHandler := handlers.NewFundingSourcesHandler(fundingSourcesSvc)
	payoutsHandler := handlers.NewPayoutsHandler(payoutsSvc)
	var outboxRelays []*outbox.Relay
	var rabbitmqClient *platformrabbitmq.Client
	if outboxRelayEnabled {
		rabbitmqClient, err = platformrabbitmq.Open(rabbitmqURL)
		if err != nil {
			log.Fatalf("open rabbitmq: %v", err)
		}
		defer rabbitmqClient.Close()

		payoutTopology := rabbitmqbroker.NewPayoutTopology(payoutQueueName)
		if err := rabbitmqClient.EnsureTopology(payoutTopology); err != nil {
			log.Fatalf("ensure payout topology: %v", err)
		}
		webhookTopology := rabbitmqbroker.NewWebhookTopology(webhookQueueName)
		if err := rabbitmqClient.EnsureTopology(webhookTopology); err != nil {
			log.Fatalf("ensure webhook topology: %v", err)
		}

		payoutRelay := outbox.NewRelay(
			outbox.NewDBTxRunner(dbPool, queries),
			rabbitmqbroker.NewPayoutPublisher(rabbitmqClient, payoutTopology.ExchangeName, payoutTopology.RoutingKey),
			log.Default(),
			outbox.Config{
				PollInterval: pollInterval,
				ClaimTimeout: claimTimeout,
				EventTypes:   []string{outbox.EventTypeProcessPayout},
			},
		)
		webhookRelay := outbox.NewRelay(
			outbox.NewDBTxRunner(dbPool, queries),
			rabbitmqbroker.NewWebhookPublisher(rabbitmqClient, webhookTopology.ExchangeName, webhookTopology.RoutingKey),
			log.Default(),
			outbox.Config{
				PollInterval: pollInterval,
				ClaimTimeout: claimTimeout,
				EventTypes:   []string{outbox.EventTypePayoutResultWebhook},
			},
		)
		outboxRelays = []*outbox.Relay{payoutRelay, webhookRelay}
	}

	router := NewRouter(clientsHandler, fundingSourcesHandler, payoutsHandler, middleware.APIKey(authSvc))

	srv := &http.Server{
		Addr:    ":8080",
		Handler: router,
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	var runOutboxRelay func(context.Context) error
	if outboxRelayEnabled {
		runOutboxRelay = func(ctx context.Context) error {
			return runOutboxRelays(ctx, outboxRelays...)
		}
		log.Printf(
			"outbox relay publisher is enabled interval=%s claim_timeout=%s payout_queue=%s webhook_queue=%s",
			pollInterval,
			claimTimeout,
			payoutQueueName,
			webhookQueueName,
		)
	} else {
		log.Println("outbox relay publisher is disabled")
	}

	log.Printf("server is running on %s", srv.Addr)
	if err := runApplication(ctx, srv, srv.ListenAndServe, runOutboxRelay, 5*time.Second, log.Default()); err != nil {
		log.Fatalf("run application: %v", err)
	}

	log.Println("closing postgres pool...")
	log.Println("server gracefully stopped")
}

func runOutboxRelays(ctx context.Context, relays ...*outbox.Relay) error {
	if len(relays) == 0 {
		return nil
	}

	relayCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	errCh := make(chan error, len(relays))
	for _, relay := range relays {
		go func(relay *outbox.Relay) {
			errCh <- normalizeOutboxRelayError(relay.Run(relayCtx))
		}(relay)
	}

	for range relays {
		if err := <-errCh; err != nil {
			cancel()
			return err
		}
	}

	return nil
}

func runApplication(
	ctx context.Context,
	srv *http.Server,
	serve func() error,
	runOutboxRelay func(context.Context) error,
	shutdownTimeout time.Duration,
	logger *log.Logger,
) error {
	if srv == nil || serve == nil {
		return errors.New("http server is not configured")
	}
	if logger == nil {
		logger = log.Default()
	}
	if shutdownTimeout <= 0 {
		shutdownTimeout = 5 * time.Second
	}

	appCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	serverDone := make(chan error, 1)
	go func() {
		serverDone <- normalizeServeError(serve())
	}()

	var outboxRelayDone chan error
	if runOutboxRelay != nil {
		outboxRelayDone = make(chan error, 1)
		go func() {
			outboxRelayDone <- normalizeOutboxRelayError(runOutboxRelay(appCtx))
		}()
	}

	var serverErr error
	serverDoneReceived := false
	var outboxRelayErr error
	outboxRelayDoneReceived := outboxRelayDone == nil

	select {
	case serverErr = <-serverDone:
		serverDoneReceived = true
	case outboxRelayErr = <-outboxRelayDone:
		outboxRelayDoneReceived = true
	case <-ctx.Done():
		logger.Println("shutting down server...")
	}

	cancel()

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer shutdownCancel()

	if err := srv.Shutdown(shutdownCtx); err != nil && !errors.Is(err, http.ErrServerClosed) {
		serverErr = firstNonNilError(serverErr, fmt.Errorf("shutdown server: %w", err))
	}

	if !serverDoneReceived {
		serverErr = firstNonNilError(serverErr, <-serverDone)
	}
	if !outboxRelayDoneReceived {
		outboxRelayErr = firstNonNilError(outboxRelayErr, <-outboxRelayDone)
	}

	if serverErr != nil {
		return serverErr
	}
	if outboxRelayErr != nil {
		return outboxRelayErr
	}

	return nil
}

func normalizeServeError(err error) error {
	if err == nil || errors.Is(err, http.ErrServerClosed) {
		return nil
	}

	return err
}

func normalizeOutboxRelayError(err error) error {
	if err == nil || errors.Is(err, context.Canceled) {
		return nil
	}

	return err
}

func firstNonNilError(errs ...error) error {
	for _, err := range errs {
		if err != nil {
			return err
		}
	}

	return nil
}

func loadBoolEnv(name string, fallback bool) (bool, error) {
	raw := os.Getenv(name)
	if raw == "" {
		return fallback, nil
	}

	value, err := strconv.ParseBool(raw)
	if err != nil {
		return false, err
	}

	return value, nil
}

func loadDurationEnv(name string, fallback time.Duration) (time.Duration, error) {
	raw := os.Getenv(name)
	if raw == "" {
		return fallback, nil
	}

	value, err := time.ParseDuration(raw)
	if err != nil {
		return 0, err
	}

	return value, nil
}

func loadStringEnv(name, fallback string) string {
	raw := os.Getenv(name)
	if raw == "" {
		return fallback
	}

	return raw
}
