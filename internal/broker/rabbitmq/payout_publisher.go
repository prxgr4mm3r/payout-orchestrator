package rabbitmq

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/prxgr4mm3r/payout-orchestrator/internal/outbox"
	platformrabbitmq "github.com/prxgr4mm3r/payout-orchestrator/internal/platform/rabbitmq"
)

type messagePublisher interface {
	Publish(ctx context.Context, exchange, routingKey string, body []byte) error
}

const (
	PayoutExchangeName = "payout.jobs"
	PayoutRoutingKey   = "payout.process"
)

type payoutJobMessage struct {
	ID        string `json:"id"`
	EventType string `json:"event_type"`
	EntityID  string `json:"entity_id"`
	Payload   []byte `json:"payload"`
}

type PayoutPublisher struct {
	publisher    messagePublisher
	exchangeName string
	routingKey   string
}

func NewPayoutPublisher(publisher messagePublisher, exchangeName, routingKey string) *PayoutPublisher {
	return &PayoutPublisher{
		publisher:    publisher,
		exchangeName: exchangeName,
		routingKey:   routingKey,
	}
}

func NewPayoutTopology(queueName string) platformrabbitmq.Topology {
	return platformrabbitmq.Topology{
		ExchangeName: PayoutExchangeName,
		QueueName:    queueName,
		RoutingKey:   PayoutRoutingKey,
	}
}

func (p *PayoutPublisher) Dispatch(ctx context.Context, event outbox.Event) error {
	if p == nil || p.publisher == nil {
		return errors.New("rabbitmq payout publisher is not configured")
	}
	if p.exchangeName == "" {
		return errors.New("rabbitmq payout exchange name is required")
	}
	if p.routingKey == "" {
		return errors.New("rabbitmq payout routing key is required")
	}

	body, err := EncodePayoutJob(event)
	if err != nil {
		return err
	}

	return p.publisher.Publish(ctx, p.exchangeName, p.routingKey, body)
}

func EncodePayoutJob(event outbox.Event) ([]byte, error) {
	return json.Marshal(payoutJobMessage{
		ID:        event.ID,
		EventType: event.EventType,
		EntityID:  event.EntityID,
		Payload:   event.Payload,
	})
}

func DecodePayoutJob(raw []byte) (outbox.Event, error) {
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
