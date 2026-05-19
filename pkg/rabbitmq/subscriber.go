package rabbitmq

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	"github.com/hosseinasadian/mini-wallet/pkg/broker"
	amqp "github.com/rabbitmq/amqp091-go"
)

// newUUID is a thin wrapper so worker.go doesn't import uuid directly.
func newUUID() string {
	return uuid.New().String()
}

// SubscriberConfig holds runtime behaviour shared by all subscriber types.
// It is consumed entirely in the constructor; no config is passed to Subscribe.
type SubscriberConfig struct {
	MaxRetry       int64
	Workers        int
	PrefetchCount  int
	HandlerTimeout time.Duration
	// OnDLQFail is called asynchronously when publishing to DLQ fails.
	// Must be non-blocking. Panics inside OnDLQFail are recovered silently.
	OnDLQFail func(msgID string, body []byte, err error)
	// OnPanic is called synchronously when a handler panics, before the nack.
	OnPanic func(recovered any, msg amqp.Delivery)
}

func (c SubscriberConfig) toWorkerConfig() workerConfig {
	return workerConfig{
		maxRetry:       c.MaxRetry,
		workers:        c.Workers,
		prefetchCount:  c.PrefetchCount,
		handlerTimeout: c.HandlerTimeout,
		onDLQFail:      c.OnDLQFail,
		onPanic:        c.OnPanic,
	}
}

// baseSubscriber holds shared state for all subscriber types.
type baseSubscriber struct {
	conn        *Connection
	publishCh   *amqp.Channel
	publishMu   sync.RWMutex
	consumers   []*consumerGroup
	consumersMu sync.Mutex
	ctx         context.Context
	cancel      context.CancelFunc
	closed      atomic.Bool
	cfg         workerConfig
}

func newBaseSubscriber(conn *Connection, cfg SubscriberConfig) (*baseSubscriber, error) {
	ch, err := conn.channel()
	if err != nil {
		return nil, fmt.Errorf("rabbitmq: open publish channel for dlq: %w", err)
	}

	ctx, cancel := context.WithCancel(context.Background())

	return &baseSubscriber{
		conn:      conn,
		publishCh: ch,
		ctx:       ctx,
		cancel:    cancel,
		cfg:       cfg.toWorkerConfig(),
	}, nil
}

func (s *baseSubscriber) subscribe(n queueNames, handler broker.Handler) error {
	s.consumersMu.Lock()
	if s.closed.Load() {
		s.consumersMu.Unlock()
		return ErrClosed
	}
	s.consumersMu.Unlock()

	cg, err := startWorkers(s.conn, s.ctx, s.publishCh, &s.publishMu, n, s.cfg, handler)
	if err != nil {
		return err
	}

	s.consumersMu.Lock()
	s.consumers = append(s.consumers, cg)
	s.consumersMu.Unlock()

	return nil
}

func (s *baseSubscriber) close(ctx context.Context) error {
	if s.closed.Swap(true) {
		return nil
	}

	s.cancel()

	s.consumersMu.Lock()
	consumers := make([]*consumerGroup, len(s.consumers))
	copy(consumers, s.consumers)
	s.consumersMu.Unlock()

	waitDone := make(chan struct{})
	go func() {
		for _, c := range consumers {
			c.wg.Wait()
		}
		close(waitDone)
	}()

	select {
	case <-waitDone:
	case <-ctx.Done():
	}

	s.publishMu.Lock()
	defer s.publishMu.Unlock()

	var errs []error

	for _, c := range consumers {
		if err := c.close(); err != nil {
			errs = append(errs, err)
		}
	}

	if err := s.publishCh.Close(); err != nil {
		errs = append(errs, fmt.Errorf("rabbitmq: close dlq publish channel: %w", err))
	}

	if len(errs) > 0 {
		// Return all errors joined; use errors.Join (Go 1.20+).
		return fmt.Errorf("rabbitmq: close subscriber: %v", errs)
	}
	return nil
}

// DirectSubscriber consumes from a direct exchange queue.
type DirectSubscriber struct {
	*baseSubscriber
	eventName string
}

func NewDirectSubscriber(conn *Connection, eventName string, cfg SubscriberConfig) (*DirectSubscriber, error) {
	base, err := newBaseSubscriber(conn, cfg)
	if err != nil {
		return nil, fmt.Errorf("rabbitmq: new direct subscriber: %w", err)
	}
	return &DirectSubscriber{baseSubscriber: base, eventName: eventName}, nil
}

func (s *DirectSubscriber) Subscribe(handler broker.Handler) error {
	n := buildNames(s.eventName, s.eventName)
	return s.subscribe(n, handler)
}

func (s *DirectSubscriber) Close(ctx context.Context) error {
	return s.close(ctx)
}

// FanoutSubscriber consumes from one queue bound to a fanout exchange.
type FanoutSubscriber struct {
	*baseSubscriber
	eventName string
	queueBase string
}

func NewFanoutSubscriber(conn *Connection, eventName, queueBase string, cfg SubscriberConfig) (*FanoutSubscriber, error) {
	base, err := newBaseSubscriber(conn, cfg)
	if err != nil {
		return nil, fmt.Errorf("rabbitmq: new fanout subscriber: %w", err)
	}
	return &FanoutSubscriber{baseSubscriber: base, eventName: eventName, queueBase: queueBase}, nil
}

func (s *FanoutSubscriber) Subscribe(handler broker.Handler) error {
	n := buildNames(s.eventName, s.queueBase)
	n.mainExchange = s.eventName + ".retry.exchange"
	return s.subscribe(n, handler)
}

func (s *FanoutSubscriber) Close(ctx context.Context) error {
	return s.close(ctx)
}

// TopicSubscriber consumes from one queue bound to a topic exchange.
type TopicSubscriber struct {
	*baseSubscriber
	eventName string
	queueBase string
}

func NewTopicSubscriber(conn *Connection, eventName, queueBase string, cfg SubscriberConfig) (*TopicSubscriber, error) {
	base, err := newBaseSubscriber(conn, cfg)
	if err != nil {
		return nil, fmt.Errorf("rabbitmq: new topic subscriber: %w", err)
	}
	return &TopicSubscriber{baseSubscriber: base, eventName: eventName, queueBase: queueBase}, nil
}

func (s *TopicSubscriber) Subscribe(handler broker.Handler) error {
	n := buildNames(s.eventName, s.queueBase)
	return s.subscribe(n, handler)
}

func (s *TopicSubscriber) Close(ctx context.Context) error {
	return s.close(ctx)
}
