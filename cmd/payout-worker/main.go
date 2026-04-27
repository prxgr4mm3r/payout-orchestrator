package main

import (
	"context"
	"errors"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	payoutworker "github.com/prxgr4mm3r/payout-orchestrator/internal/apps/payoutworker"
	"github.com/prxgr4mm3r/payout-orchestrator/internal/apps/payoutworker/execution"
	"github.com/prxgr4mm3r/payout-orchestrator/internal/db"
	"github.com/prxgr4mm3r/payout-orchestrator/internal/platform/postgres"
	platformrabbitmq "github.com/prxgr4mm3r/payout-orchestrator/internal/platform/rabbitmq"
	"github.com/prxgr4mm3r/payout-orchestrator/internal/providersimulator"
)

func main() {
	dbURL := os.Getenv("DB_URL")
	rabbitmqURL := os.Getenv("RABBITMQ_URL")
	payoutQueueName := loadStringEnv("PAYOUT_QUEUE_NAME", "payout.jobs")

	startupCtx, startupCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer startupCancel()

	dbPool, err := postgres.Open(startupCtx, dbURL)
	if err != nil {
		log.Fatalf("open postgres: %v", err)
	}
	defer dbPool.Close()

	rabbitmqClient, err := platformrabbitmq.Open(rabbitmqURL)
	if err != nil {
		log.Fatalf("open rabbitmq: %v", err)
	}
	defer rabbitmqClient.Close()

	if err := rabbitmqClient.EnsureQueue(payoutQueueName); err != nil {
		log.Fatalf("ensure payout queue: %v", err)
	}

	queries := db.New(dbPool)
	worker := payoutworker.New(
		rabbitmqClient,
		execution.NewHandler(
			execution.NewDBTxRunner(dbPool, queries),
			providersimulator.New(providersimulator.Config{}),
			log.Default(),
		),
		payoutQueueName,
		log.Default(),
	)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	log.Printf("payout worker is running queue=%s", payoutQueueName)
	if err := worker.Run(ctx); err != nil && !errors.Is(err, context.Canceled) {
		log.Fatalf("run payout worker: %v", err)
	}
	log.Println("payout worker stopped")
}

func loadStringEnv(name, fallback string) string {
	raw := os.Getenv(name)
	if raw == "" {
		return fallback
	}

	return raw
}
