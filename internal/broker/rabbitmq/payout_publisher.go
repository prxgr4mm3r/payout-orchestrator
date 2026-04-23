package rabbitmq

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/prxgr4mm3r/payout-orchestrator/internal/outbox"
)

type messagePublisher interface {
	Publish(ctx context.Context, queue string, body []byte) error
}

type payoutJobMessage struct {
	ID        string `json:"id"`
	EventType string `json:"event_type"`
	EntityID  string `json:"entity_id"`
	Payload   []byte `json:"payload"`
}

type PayoutPublisher struct {
	publisher messagePublisher
	queueName string
}

func NewPayoutPublisher(publisher messagePublisher, queueName string) *PayoutPublisher {
	return &PayoutPublisher{
		publisher: publisher,
		queueName: queueName,
	}
}

func (p *PayoutPublisher) Dispatch(ctx context.Context, event outbox.Event) error {
	if p == nil || p.publisher == nil {
		return errors.New("rabbitmq payout publisher is not configured")
	}
	if p.queueName == "" {
		return errors.New("rabbitmq payout queue name is required")
	}

	body, err := json.Marshal(payoutJobMessage{
		ID:        event.ID,
		EventType: event.EventType,
		EntityID:  event.EntityID,
		Payload:   event.Payload,
	})
	if err != nil {
		return err
	}

	return p.publisher.Publish(ctx, p.queueName, body)
}

func decodePayoutJob(raw []byte) (outbox.Event, error) {
	var message payoutJobMessage
	if err := json.Unmarshal(raw, &message); err != nil {
		return outbox.Event{}, err
	}

	return outbox.Event{
		ID:        message.ID,
		EventType: message.EventType,
		EntityID:  message.EntityID,
		Payload:   message.Payload,
	}, nil
}
