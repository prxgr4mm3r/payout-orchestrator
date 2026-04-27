package rabbitmq

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/prxgr4mm3r/payout-orchestrator/internal/outbox"
	platformrabbitmq "github.com/prxgr4mm3r/payout-orchestrator/internal/platform/rabbitmq"
)

const (
	WebhookExchangeName = "webhook.deliveries"
	WebhookRoutingKey   = "webhook.deliver"
)

type webhookDeliveryMessage struct {
	ID        string `json:"id"`
	EventType string `json:"event_type"`
	EntityID  string `json:"entity_id"`
	Payload   []byte `json:"payload"`
}

type WebhookPublisher struct {
	publisher    messagePublisher
	exchangeName string
	routingKey   string
}

func NewWebhookPublisher(publisher messagePublisher, exchangeName, routingKey string) *WebhookPublisher {
	return &WebhookPublisher{
		publisher:    publisher,
		exchangeName: exchangeName,
		routingKey:   routingKey,
	}
}

func NewWebhookTopology(queueName string) platformrabbitmq.Topology {
	return platformrabbitmq.Topology{
		ExchangeName: WebhookExchangeName,
		QueueName:    queueName,
		RoutingKey:   WebhookRoutingKey,
	}
}

func (p *WebhookPublisher) Dispatch(ctx context.Context, event outbox.Event) error {
	if p == nil || p.publisher == nil {
		return errors.New("rabbitmq webhook publisher is not configured")
	}
	if p.exchangeName == "" {
		return errors.New("rabbitmq webhook exchange name is required")
	}
	if p.routingKey == "" {
		return errors.New("rabbitmq webhook routing key is required")
	}

	body, err := EncodeWebhookDeliveryJob(event)
	if err != nil {
		return err
	}

	return p.publisher.Publish(ctx, p.exchangeName, p.routingKey, body)
}

func EncodeWebhookDeliveryJob(event outbox.Event) ([]byte, error) {
	return json.Marshal(webhookDeliveryMessage{
		ID:        event.ID,
		EventType: event.EventType,
		EntityID:  event.EntityID,
		Payload:   event.Payload,
	})
}

func DecodeWebhookDeliveryJob(raw []byte) (outbox.Event, error) {
	var message webhookDeliveryMessage
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
