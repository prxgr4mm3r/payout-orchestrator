package rabbitmq

import (
	"context"
	"errors"

	amqp "github.com/rabbitmq/amqp091-go"
)

type Delivery interface {
	Body() []byte
	Ack() error
	Nack(requeue bool) error
}

type Client struct {
	conn    *amqp.Connection
	channel *amqp.Channel
}

func Open(url string) (*Client, error) {
	if url == "" {
		return nil, errors.New("rabbitmq url is required")
	}

	conn, err := amqp.Dial(url)
	if err != nil {
		return nil, err
	}

	channel, err := conn.Channel()
	if err != nil {
		_ = conn.Close()
		return nil, err
	}

	return &Client{
		conn:    conn,
		channel: channel,
	}, nil
}

func (c *Client) Close() error {
	if c == nil {
		return nil
	}

	var closeErr error
	if c.channel != nil {
		closeErr = c.channel.Close()
	}
	if c.conn != nil {
		if err := c.conn.Close(); err != nil && closeErr == nil {
			closeErr = err
		}
	}

	return closeErr
}

func (c *Client) EnsureQueue(name string) error {
	if c == nil || c.channel == nil {
		return errors.New("rabbitmq client is not configured")
	}
	if name == "" {
		return errors.New("rabbitmq queue name is required")
	}

	_, err := c.channel.QueueDeclare(
		name,
		true,
		false,
		false,
		false,
		nil,
	)

	return err
}

func (c *Client) Publish(ctx context.Context, queue string, body []byte) error {
	if c == nil || c.channel == nil {
		return errors.New("rabbitmq client is not configured")
	}
	if queue == "" {
		return errors.New("rabbitmq queue name is required")
	}

	return c.channel.PublishWithContext(
		ctx,
		"",
		queue,
		false,
		false,
		amqp.Publishing{
			ContentType:  "application/json",
			Body:         body,
			DeliveryMode: amqp.Persistent,
		},
	)
}

func (c *Client) Consume(ctx context.Context, queue string, handler func(context.Context, Delivery) error) error {
	if c == nil || c.channel == nil {
		return errors.New("rabbitmq client is not configured")
	}
	if queue == "" {
		return errors.New("rabbitmq queue name is required")
	}
	if handler == nil {
		return errors.New("rabbitmq consumer handler is not configured")
	}

	deliveries, err := c.channel.Consume(
		queue,
		"",
		false,
		false,
		false,
		false,
		nil,
	)
	if err != nil {
		return err
	}

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case message, ok := <-deliveries:
			if !ok {
				return errors.New("rabbitmq deliveries channel closed")
			}
			if err := handler(ctx, amqpDelivery{delivery: message}); err != nil {
				return err
			}
		}
	}
}

type amqpDelivery struct {
	delivery amqp.Delivery
}

func (d amqpDelivery) Body() []byte {
	return d.delivery.Body
}

func (d amqpDelivery) Ack() error {
	return d.delivery.Ack(false)
}

func (d amqpDelivery) Nack(requeue bool) error {
	return d.delivery.Nack(false, requeue)
}
