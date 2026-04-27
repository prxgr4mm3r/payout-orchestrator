package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	webhookworker "github.com/prxgr4mm3r/payout-orchestrator/internal/apps/webhookworker"
	"github.com/prxgr4mm3r/payout-orchestrator/internal/apps/webhookworker/delivery"
	rabbitmqbroker "github.com/prxgr4mm3r/payout-orchestrator/internal/broker/rabbitmq"
	"github.com/prxgr4mm3r/payout-orchestrator/internal/db"
	"github.com/prxgr4mm3r/payout-orchestrator/internal/platform/postgres"
	platformrabbitmq "github.com/prxgr4mm3r/payout-orchestrator/internal/platform/rabbitmq"
)

func main() {
	dbURL := os.Getenv("DB_URL")
	rabbitmqURL := os.Getenv("RABBITMQ_URL")
	webhookQueueName := loadStringEnv("WEBHOOK_QUEUE_NAME", "webhook.deliveries")

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

	webhookTopology := rabbitmqbroker.NewWebhookTopology(webhookQueueName)
	if err := rabbitmqClient.EnsureTopology(webhookTopology); err != nil {
		log.Fatalf("ensure webhook topology: %v", err)
	}

	queries := db.New(dbPool)
	worker := webhookworker.New(
		rabbitmqClient,
		delivery.NewService(queries, http.DefaultClient, log.Default()),
		webhookQueueName,
		log.Default(),
	)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	log.Printf("webhook worker is running queue=%s", webhookQueueName)
	if err := worker.Run(ctx); err != nil && !errors.Is(err, context.Canceled) {
		log.Fatalf("run webhook worker: %v", err)
	}
	log.Println("webhook worker stopped")
}

func loadStringEnv(name, fallback string) string {
	raw := os.Getenv(name)
	if raw == "" {
		return fallback
	}

	return raw
}
