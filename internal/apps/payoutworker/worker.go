package payoutworker

import (
	"context"
	"errors"
	"log"

	rabbitmqbroker "github.com/prxgr4mm3r/payout-orchestrator/internal/broker/rabbitmq"
	"github.com/prxgr4mm3r/payout-orchestrator/internal/outbox"
	platformrabbitmq "github.com/prxgr4mm3r/payout-orchestrator/internal/platform/rabbitmq"
)

type messageConsumer interface {
	Consume(ctx context.Context, queue string, handler func(context.Context, platformrabbitmq.Delivery) error) error
}

type Worker struct {
	consumer  messageConsumer
	handler   outbox.EventHandler
	queueName string
	logger    *log.Logger
}

func New(consumer messageConsumer, handler outbox.EventHandler, queueName string, logger *log.Logger) *Worker {
	if logger == nil {
		logger = log.Default()
	}

	return &Worker{
		consumer:  consumer,
		handler:   handler,
		queueName: queueName,
		logger:    logger,
	}
}

func (w *Worker) Run(ctx context.Context) error {
	if w == nil || w.consumer == nil || w.handler == nil {
		return errors.New("payout worker is not configured")
	}
	if w.queueName == "" {
		return errors.New("payout worker queue name is required")
	}

	return w.consumer.Consume(ctx, w.queueName, w.handleDelivery)
}

func (w *Worker) handleDelivery(ctx context.Context, delivery platformrabbitmq.Delivery) error {
	event, err := rabbitmqbroker.DecodePayoutJob(delivery.Body())
	if err != nil {
		if nackErr := delivery.Nack(false); nackErr != nil {
			return nackErr
		}

		w.logger.Printf("dropped malformed payout message: %v", err)
		return nil
	}

	if err := w.handler.HandleEvent(ctx, event); err != nil {
		if nackErr := delivery.Nack(true); nackErr != nil {
			return nackErr
		}

		w.logger.Printf("payout message handling failed event_id=%s type=%s err=%v", event.ID, event.EventType, err)
		return nil
	}

	if err := delivery.Ack(); err != nil {
		return err
	}

	w.logger.Printf("processed payout message event_id=%s type=%s", event.ID, event.EventType)
	return nil
}
