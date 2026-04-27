package rabbitmq

import (
	"context"
	"testing"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
)

func TestWaitForPublishConfirmAcceptsAck(t *testing.T) {
	t.Parallel()

	confirmations := make(chan amqp.Confirmation, 1)
	confirmations <- amqp.Confirmation{DeliveryTag: 1, Ack: true}

	client := &Client{
		publishConfirmTimeout: 10 * time.Millisecond,
		publishConfirmations:  confirmations,
		publishReturns:        make(chan amqp.Return),
	}

	if err := client.waitForPublishConfirm(context.Background()); err != nil {
		t.Fatalf("wait for publish confirm: %v", err)
	}
}

func TestWaitForPublishConfirmRejectsNack(t *testing.T) {
	t.Parallel()

	confirmations := make(chan amqp.Confirmation, 1)
	confirmations <- amqp.Confirmation{DeliveryTag: 2, Ack: false}

	client := &Client{
		publishConfirmTimeout: 10 * time.Millisecond,
		publishConfirmations:  confirmations,
		publishReturns:        make(chan amqp.Return),
	}

	if err := client.waitForPublishConfirm(context.Background()); err == nil {
		t.Fatal("expected publish nack error")
	}
}

func TestWaitForPublishConfirmRejectsReturnedMessage(t *testing.T) {
	t.Parallel()

	confirmations := make(chan amqp.Confirmation)
	returns := make(chan amqp.Return, 1)
	returns <- amqp.Return{ReplyCode: 312, ReplyText: "NO_ROUTE"}

	client := &Client{
		publishConfirmTimeout: 10 * time.Millisecond,
		publishConfirmations:  confirmations,
		publishReturns:        returns,
	}

	go func() {
		time.Sleep(time.Millisecond)
		confirmations <- amqp.Confirmation{DeliveryTag: 3, Ack: true}
	}()

	if err := client.waitForPublishConfirm(context.Background()); err == nil {
		t.Fatal("expected returned publish error")
	}
}
