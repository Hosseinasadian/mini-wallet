package rabbitmq

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/hosseinasadian/mini-wallet/pkg/broker"
	amqp "github.com/rabbitmq/amqp091-go"
)

type workerConfig struct {
	maxRetry       int64
	workers        int
	prefetchCount  int
	handlerTimeout time.Duration
	onDLQFail      func(msgID string, body []byte, err error)
	onPanic        func(recovered any, msg amqp.Delivery)
}

type consumerGroup struct {
	ch  *amqp.Channel
	tag string
	wg  sync.WaitGroup
}

func (cg *consumerGroup) close() error {
	var errs []error
	if err := cg.ch.Cancel(cg.tag, false); err != nil {
		errs = append(errs, fmt.Errorf("rabbitmq: cancel consumer %s: %w", cg.tag, err))
	}
	if err := cg.ch.Close(); err != nil {
		errs = append(errs, fmt.Errorf("rabbitmq: close consumer channel %s: %w", cg.tag, err))
	}
	if len(errs) > 0 {
		return errs[0]
	}
	return nil
}

func startWorkers(
	conn *Connection,
	ctx context.Context,
	publishCh *amqp.Channel,
	publishMu *sync.RWMutex,
	n queueNames,
	cfg workerConfig,
	handler broker.Handler,
) (*consumerGroup, error) {
	workers := cfg.workers
	if workers <= 0 {
		workers = 1
	}

	timeout := cfg.handlerTimeout
	if timeout <= 0 {
		timeout = 30 * time.Second
	}

	prefetch := cfg.prefetchCount
	if prefetch <= 0 {
		prefetch = 10
	}

	ch, err := conn.channel()
	if err != nil {
		return nil, fmt.Errorf("rabbitmq: open consume channel for %s: %w", n.mainQueue, err)
	}

	if err := ch.Qos(prefetch, 0, false); err != nil {
		_ = ch.Close()
		return nil, fmt.Errorf("rabbitmq: set qos for %s: %w", n.mainQueue, err)
	}

	tag := newUUID()

	msgs, err := ch.Consume(n.mainQueue, tag, false, false, false, false, nil)
	if err != nil {
		_ = ch.Close()
		return nil, fmt.Errorf("rabbitmq: consume %s: %w", n.mainQueue, err)
	}

	cg := &consumerGroup{ch: ch, tag: tag}

	for i := 0; i < workers; i++ {
		cg.wg.Add(1)
		go runWorker(cg, ctx, msgs, publishCh, publishMu, cfg, n, handler, timeout)
	}

	return cg, nil
}

func runWorker(
	cg *consumerGroup,
	ctx context.Context,
	msgs <-chan amqp.Delivery,
	publishCh *amqp.Channel,
	publishMu *sync.RWMutex,
	cfg workerConfig,
	n queueNames,
	handler broker.Handler,
	timeout time.Duration,
) {
	defer cg.wg.Done()
	for {
		select {
		case <-ctx.Done():
			return
		case msg, ok := <-msgs:
			if !ok {
				return
			}
			processMessage(msg, publishCh, publishMu, cfg, n, handler, timeout)
		}
	}
}

func processMessage(
	msg amqp.Delivery,
	publishCh *amqp.Channel,
	publishMu *sync.RWMutex,
	cfg workerConfig,
	n queueNames,
	handler broker.Handler,
	timeout time.Duration,
) {
	defer func() {
		if rec := recover(); rec != nil {
			if cfg.onPanic != nil {
				cfg.onPanic(rec, msg)
			}
			_ = msg.Nack(false, true)
		}
	}()

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	err := handler(ctx, broker.Message{
		ID:        msg.MessageId,
		Body:      msg.Body,
		Headers:   msg.Headers,
		Timestamp: msg.Timestamp,
	})

	if err == nil {
		_ = msg.Ack(false)
		return
	}

	if extractRejectedCount(msg.Headers, n.mainQueue) >= cfg.maxRetry {
		publishMu.RLock()
		pubErr := publishCh.PublishWithContext(
			context.Background(),
			n.dlxExchange,
			n.dlq,
			false,
			false,
			amqp.Publishing{
				ContentType:  msg.ContentType,
				Body:         msg.Body,
				DeliveryMode: amqp.Persistent,
				MessageId:    msg.MessageId,
				Timestamp:    msg.Timestamp,
				Headers:      msg.Headers,
			},
		)
		publishMu.RUnlock()

		if pubErr != nil {
			if cfg.onDLQFail != nil {
				go func() {
					defer func() { recover() }()
					cfg.onDLQFail(msg.MessageId, msg.Body, pubErr)
				}()
			}
			_ = msg.Nack(false, true)
			return
		}

		_ = msg.Ack(false)
		return
	}

	_ = msg.Nack(false, false)
}
