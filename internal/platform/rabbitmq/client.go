package rabbitmq

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
)

const defaultPublishConfirmTimeout = 5 * time.Second

type Delivery interface {
	Body() []byte
	Ack() error
	Nack(requeue bool) error
}

type Topology struct {
	ExchangeName string
	QueueName    string
	RoutingKey   string
}

type Client struct {
	conn                  *amqp.Connection
	channel               *amqp.Channel
	publishConfirmTimeout time.Duration
	publishConfirmations  <-chan amqp.Confirmation
	publishReturns        <-chan amqp.Return
	publishMu             sync.Mutex
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

	if err := channel.Confirm(false); err != nil {
		_ = channel.Close()
		_ = conn.Close()
		return nil, err
	}

	return &Client{
		conn:                  conn,
		channel:               channel,
		publishConfirmTimeout: defaultPublishConfirmTimeout,
		publishConfirmations:  channel.NotifyPublish(make(chan amqp.Confirmation, 1)),
		publishReturns:        channel.NotifyReturn(make(chan amqp.Return, 1)),
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

func (c *Client) EnsureTopology(topology Topology) error {
	if c == nil || c.channel == nil {
		return errors.New("rabbitmq client is not configured")
	}
	if topology.ExchangeName == "" {
		return errors.New("rabbitmq exchange name is required")
	}
	if topology.QueueName == "" {
		return errors.New("rabbitmq queue name is required")
	}
	if topology.RoutingKey == "" {
		return errors.New("rabbitmq routing key is required")
	}

	if err := c.channel.ExchangeDeclare(
		topology.ExchangeName,
		amqp.ExchangeDirect,
		true,
		false,
		false,
		false,
		nil,
	); err != nil {
		return err
	}

	if err := c.EnsureQueue(topology.QueueName); err != nil {
		return err
	}

	return c.channel.QueueBind(
		topology.QueueName,
		topology.RoutingKey,
		topology.ExchangeName,
		false,
		nil,
	)
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

func (c *Client) Publish(ctx context.Context, exchange, routingKey string, body []byte) error {
	if c == nil || c.channel == nil {
		return errors.New("rabbitmq client is not configured")
	}
	if exchange == "" {
		return errors.New("rabbitmq exchange name is required")
	}
	if routingKey == "" {
		return errors.New("rabbitmq routing key is required")
	}
	if c.publishConfirmations == nil {
		return errors.New("rabbitmq publish confirmations are not configured")
	}
	if c.publishReturns == nil {
		return errors.New("rabbitmq publish returns are not configured")
	}

	c.publishMu.Lock()
	defer c.publishMu.Unlock()

	if err := c.channel.PublishWithContext(
		ctx,
		exchange,
		routingKey,
		true,
		false,
		amqp.Publishing{
			ContentType:  "application/json",
			Body:         body,
			DeliveryMode: amqp.Persistent,
		},
	); err != nil {
		return err
	}

	return c.waitForPublishConfirm(ctx)
}

func (c *Client) waitForPublishConfirm(ctx context.Context) error {
	timeout := c.publishConfirmTimeout
	if timeout <= 0 {
		timeout = defaultPublishConfirmTimeout
	}

	timer := time.NewTimer(timeout)
	defer timer.Stop()

	var returnErr error
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return errors.New("rabbitmq publish confirmation timed out")
	case returned, ok := <-c.publishReturns:
		if !ok {
			return errors.New("rabbitmq publish returns channel closed")
		}
		returnErr = fmt.Errorf("rabbitmq publish was returned reply_code=%d reply_text=%q", returned.ReplyCode, returned.ReplyText)
	case confirmation, ok := <-c.publishConfirmations:
		if !ok {
			return errors.New("rabbitmq publish confirmations channel closed")
		}
		if !confirmation.Ack {
			return fmt.Errorf("rabbitmq publish was negatively acknowledged delivery_tag=%d", confirmation.DeliveryTag)
		}

		return nil
	}

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return errors.New("rabbitmq publish confirmation timed out")
	case confirmation, ok := <-c.publishConfirmations:
		if !ok {
			return errors.New("rabbitmq publish confirmations channel closed")
		}
		if !confirmation.Ack {
			return fmt.Errorf("rabbitmq publish was negatively acknowledged delivery_tag=%d", confirmation.DeliveryTag)
		}

		return returnErr
	}
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
