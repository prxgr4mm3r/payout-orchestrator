package rabbitmq

import (
	"context"
	"errors"
	"log"

	"github.com/prxgr4mm3r/payout-orchestrator/internal/outbox"
)

type messageDelivery interface {
	Body() []byte
	Ack() error
	Nack(requeue bool) error
}

type messageConsumer interface {
	Consume(ctx context.Context, queue string, handler func(context.Context, Delivery) error) error
}

type PayoutWorker struct {
	consumer  messageConsumer
	handler   outbox.EventHandler
	queueName string
	logger    *log.Logger
}

func NewPayoutWorker(consumer messageConsumer, handler outbox.EventHandler, queueName string, logger *log.Logger) *PayoutWorker {
	if logger == nil {
		logger = log.Default()
	}

	return &PayoutWorker{
		consumer:  consumer,
		handler:   handler,
		queueName: queueName,
		logger:    logger,
	}
}

func (w *PayoutWorker) Run(ctx context.Context) error {
	if w == nil || w.consumer == nil || w.handler == nil {
		return errors.New("rabbitmq payout worker is not configured")
	}
	if w.queueName == "" {
		return errors.New("rabbitmq payout queue name is required")
	}

	return w.consumer.Consume(ctx, w.queueName, w.handleDelivery)
}

func (w *PayoutWorker) handleDelivery(ctx context.Context, delivery Delivery) error {
	event, err := decodePayoutJob(delivery.Body())
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
