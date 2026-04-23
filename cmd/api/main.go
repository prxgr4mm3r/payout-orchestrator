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

	"github.com/prxgr4mm3r/payout-orchestrator/internal/api/handlers"
	"github.com/prxgr4mm3r/payout-orchestrator/internal/api/middleware"
	authservice "github.com/prxgr4mm3r/payout-orchestrator/internal/api/services/auth"
	fundingservice "github.com/prxgr4mm3r/payout-orchestrator/internal/api/services/fundingsources"
	payoutservice "github.com/prxgr4mm3r/payout-orchestrator/internal/api/services/payouts"
	rabbitmqbroker "github.com/prxgr4mm3r/payout-orchestrator/internal/broker/rabbitmq"
	"github.com/prxgr4mm3r/payout-orchestrator/internal/db"
	"github.com/prxgr4mm3r/payout-orchestrator/internal/outbox"
	"github.com/prxgr4mm3r/payout-orchestrator/internal/platform/postgres"
)

func main() {
	dbURL := os.Getenv("DB_URL")
	rabbitmqURL := os.Getenv("RABBITMQ_URL")
	payoutQueueName := loadStringEnv("PAYOUT_QUEUE_NAME", "payout.jobs")
	processorEnabled, err := loadBoolEnv("PROCESSOR_ENABLED", false)
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
	var outboxRelay *outbox.Relay
	var rabbitmqClient *rabbitmqbroker.Client
	if processorEnabled {
		rabbitmqClient, err = rabbitmqbroker.Open(rabbitmqURL)
		if err != nil {
			log.Fatalf("open rabbitmq: %v", err)
		}
		defer rabbitmqClient.Close()

		if err := rabbitmqClient.EnsureQueue(payoutQueueName); err != nil {
			log.Fatalf("ensure payout queue: %v", err)
		}

		outboxRelay = outbox.NewRelay(
			outbox.NewDBTxRunner(dbPool, queries),
			rabbitmqbroker.NewPayoutPublisher(rabbitmqClient, payoutQueueName),
			log.Default(),
			outbox.Config{
				PollInterval: pollInterval,
				ClaimTimeout: claimTimeout,
			},
		)
	}

	router := NewRouter(clientsHandler, fundingSourcesHandler, payoutsHandler, middleware.APIKey(authSvc))

	srv := &http.Server{
		Addr:    ":8080",
		Handler: router,
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	var runProcessor func(context.Context) error
	if processorEnabled {
		runProcessor = outboxRelay.Run
		log.Printf(
			"outbox relay publisher is enabled interval=%s claim_timeout=%s queue=%s",
			pollInterval,
			claimTimeout,
			payoutQueueName,
		)
	} else {
		log.Println("outbox relay publisher is disabled")
	}

	log.Printf("server is running on %s", srv.Addr)
	if err := runApplication(ctx, srv, srv.ListenAndServe, runProcessor, 5*time.Second, log.Default()); err != nil {
		log.Fatalf("run application: %v", err)
	}

	log.Println("closing postgres pool...")
	log.Println("server gracefully stopped")
}

func runApplication(
	ctx context.Context,
	srv *http.Server,
	serve func() error,
	runProcessor func(context.Context) error,
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

	var processorDone chan error
	if runProcessor != nil {
		processorDone = make(chan error, 1)
		go func() {
			processorDone <- normalizeProcessorError(runProcessor(appCtx))
		}()
	}

	var serverErr error
	serverDoneReceived := false
	var processorErr error
	processorDoneReceived := processorDone == nil

	select {
	case serverErr = <-serverDone:
		serverDoneReceived = true
	case processorErr = <-processorDone:
		processorDoneReceived = true
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
	if !processorDoneReceived {
		processorErr = firstNonNilError(processorErr, <-processorDone)
	}

	if serverErr != nil {
		return serverErr
	}
	if processorErr != nil {
		return processorErr
	}

	return nil
}

func normalizeServeError(err error) error {
	if err == nil || errors.Is(err, http.ErrServerClosed) {
		return nil
	}

	return err
}

func normalizeProcessorError(err error) error {
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
