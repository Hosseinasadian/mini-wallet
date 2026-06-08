package rabbitmq

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	amqp "github.com/rabbitmq/amqp091-go"
)

type basePublisher struct {
	conn   *Connection
	ch     *amqp.Channel
	mu     sync.RWMutex
	closed atomic.Bool
}

func newBasePublisher(conn *Connection) (*basePublisher, error) {
	ch, err := conn.channel()
	if err != nil {
		return nil, err
	}
	return &basePublisher{conn: conn, ch: ch}, nil
}

func (p *basePublisher) close() error {
	if p.closed.Swap(true) {
		return nil
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if err := p.ch.Close(); err != nil {
		return fmt.Errorf("rabbitmq: close publish channel: %w", err)
	}
	return nil
}

func (p *basePublisher) publish(ctx context.Context, exchange, routingKey string, body []byte) error {
	p.mu.RLock()
	defer p.mu.RUnlock()

	if p.closed.Load() {
		return ErrClosed
	}

	err := p.ch.PublishWithContext(
		ctx,
		exchange,
		routingKey,
		false,
		false,
		amqp.Publishing{
			ContentType:  "application/json",
			Body:         body,
			DeliveryMode: amqp.Persistent,
			MessageId:    uuid.New().String(),
			Timestamp:    time.Now(),
		},
	)

	if err != nil {
		if !isChannelError(err) {
			return err
		}

		newCh, chErr := p.conn.channel()
		if chErr != nil {
			return fmt.Errorf("rabbitmq: reopen channel: %w", chErr)
		}
		p.ch = newCh

		return p.ch.PublishWithContext(
			ctx,
			exchange,
			routingKey,
			false,
			false,
			amqp.Publishing{
				ContentType:  "application/json",
				Body:         body,
				DeliveryMode: amqp.Persistent,
				MessageId:    uuid.New().String(),
				Timestamp:    time.Now(),
			},
		)
	}

	return nil
}

// DirectPublisher publishes to 1780772965_create_notification_outbox_events_table.up.sql direct exchange.
type DirectPublisher struct {
	*basePublisher
	eventName string
}

func NewDirectPublisher(conn *Connection, eventName string) (*DirectPublisher, error) {
	base, err := newBasePublisher(conn)
	if err != nil {
		return nil, fmt.Errorf("rabbitmq: new direct publisher: %w", err)
	}
	return &DirectPublisher{basePublisher: base, eventName: eventName}, nil
}

func (p *DirectPublisher) Publish(ctx context.Context, body []byte) error {
	n := buildNames(p.eventName, p.eventName)

	return p.publish(
		ctx,
		n.mainExchange,
		n.mainQueue,
		body,
	)
}

func (p *DirectPublisher) Close() error {
	return p.close()
}

// FanoutPublisher publishes to 1780772965_create_notification_outbox_events_table.up.sql fanout exchange.
type FanoutPublisher struct {
	*basePublisher
	eventName string
}

func NewFanoutPublisher(conn *Connection, eventName string) (*FanoutPublisher, error) {
	base, err := newBasePublisher(conn)
	if err != nil {
		return nil, fmt.Errorf("rabbitmq: new fanout publisher: %w", err)
	}
	return &FanoutPublisher{basePublisher: base, eventName: eventName}, nil
}

func (p *FanoutPublisher) Publish(ctx context.Context, body []byte) error {
	return p.publish(
		ctx,
		p.eventName+".exchange",
		"",
		body,
	)
}

func (p *FanoutPublisher) Close() error {
	return p.close()
}

// TopicPublisher publishes to 1780772965_create_notification_outbox_events_table.up.sql topic exchange with 1780772965_create_notification_outbox_events_table.up.sql routing key.
type TopicPublisher struct {
	*basePublisher
	eventName string
}

func NewTopicPublisher(conn *Connection, eventName string) (*TopicPublisher, error) {
	base, err := newBasePublisher(conn)
	if err != nil {
		return nil, fmt.Errorf("rabbitmq: new topic publisher: %w", err)
	}
	return &TopicPublisher{basePublisher: base, eventName: eventName}, nil
}

func (p *TopicPublisher) Publish(ctx context.Context, routingKey string, body []byte) error {
	return p.publish(
		ctx,
		p.eventName+".exchange",
		routingKey,
		body,
	)
}

func (p *TopicPublisher) Close() error {
	return p.close()
}
