package main

import (
	"context"
	"errors"
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
	"github.com/prxgr4mm3r/payout-orchestrator/internal/db"
	"github.com/prxgr4mm3r/payout-orchestrator/internal/platform/postgres"
	"github.com/prxgr4mm3r/payout-orchestrator/internal/processor"
	"github.com/prxgr4mm3r/payout-orchestrator/internal/providersimulator"
)

func main() {
	dbURL := os.Getenv("DB_URL")
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
	payoutProcessor := processor.New(
		processor.NewDBTxRunner(dbPool, queries),
		processor.NewExecutionHandler(providersimulator.New(providersimulator.Config{})),
		log.Default(),
		processor.Config{
			PollInterval: pollInterval,
			ClaimTimeout: claimTimeout,
		},
	)

	router := NewRouter(clientsHandler, fundingSourcesHandler, payoutsHandler, middleware.APIKey(authSvc))

	srv := &http.Server{
		Addr:    ":8080",
		Handler: router,
	}

	errCh := make(chan error, 1)

	go func() {
		log.Printf("server is running on %s", srv.Addr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			errCh <- err
		}

		close(errCh)
	}()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	var processorErrCh chan error
	if processorEnabled {
		processorErrCh = make(chan error, 1)
		go func() {
			if err := payoutProcessor.Run(ctx); err != nil && !errors.Is(err, context.Canceled) {
				processorErrCh <- err
			}

			close(processorErrCh)
		}()
		log.Printf("background processor is enabled interval=%s claim_timeout=%s", pollInterval, claimTimeout)
	} else {
		log.Println("background processor is disabled")
	}

	select {
	case err := <-errCh:
		if err != nil {
			log.Fatalf("server error: %v", err)
		}
	case err := <-processorErrCh:
		if err != nil {
			log.Fatalf("processor error: %v", err)
		}
	case <-ctx.Done():
		log.Println("shutting down server...")
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Fatalf("server shutdown error: %v", err)
	}

	log.Println("closing postgres pool...")
	log.Println("server gracefully stopped")
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
