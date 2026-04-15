package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/prxgr4mm3r/payout-orchestrator/internal/api/handlers"
	"github.com/prxgr4mm3r/payout-orchestrator/internal/api/middleware"
	authservice "github.com/prxgr4mm3r/payout-orchestrator/internal/api/services/auth"
	fundingservice "github.com/prxgr4mm3r/payout-orchestrator/internal/api/services/fundingsources"
	"github.com/prxgr4mm3r/payout-orchestrator/internal/db"
	"github.com/prxgr4mm3r/payout-orchestrator/internal/platform/postgres"
)

func main() {
	dbURL := os.Getenv("DB_URL")

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
	clientsHandler := &handlers.ClientsHandler{}
	fundingSourcesHandler := handlers.NewFundingSourcesHandler(fundingSourcesSvc)

	router := NewRouter(clientsHandler, fundingSourcesHandler, middleware.APIKey(authSvc))

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

	select {
	case err := <-errCh:
		if err != nil {
			log.Fatalf("server error: %v", err)
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
