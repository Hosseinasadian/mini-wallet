package rabbitmq

import (
	"fmt"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
)

type DirectTopologyConfig struct {
	EventName string
	RetryTTL  time.Duration
}

type FanoutTopologyConfig struct {
	EventName string
	RetryTTL  time.Duration
	Queues    []string
}

type TopicTopologyConfig struct {
	EventName string
	RetryTTL  time.Duration
	Bindings  []TopicBinding
}

type TopicBinding struct {
	Queue      string
	RoutingKey string
}

type Topology struct {
	ch *amqp.Channel
}

func NewTopology(conn *Connection) (*Topology, error) {
	ch, err := conn.channel()
	if err != nil {
		return nil, fmt.Errorf("rabbitmq: open topology channel: %w", err)
	}
	return &Topology{ch: ch}, nil
}

func (t *Topology) Close() error {
	if err := t.ch.Close(); err != nil {
		return fmt.Errorf("rabbitmq: close topology channel: %w", err)
	}
	return nil
}

func (t *Topology) DeclareDirect(cfg DirectTopologyConfig) error {
	n := buildNames(cfg.EventName, cfg.EventName)

	if err := t.declareExchange(n.mainExchange, "direct"); err != nil {
		return err
	}
	if err := t.declareExchange(n.dlxExchange, "direct"); err != nil {
		return err
	}
	if err := t.declareQueueWithDLX(n.mainQueue, n.mainExchange, n.retryQueue); err != nil {
		return err
	}
	if err := t.declareRetryQueue(n.retryQueue, n.mainExchange, n.mainQueue, cfg.RetryTTL); err != nil {
		return err
	}
	if err := t.declareDLQ(n.dlq); err != nil {
		return err
	}

	for _, b := range []struct{ queue, key, exchange string }{
		{n.mainQueue, n.mainQueue, n.mainExchange},
		{n.retryQueue, n.retryQueue, n.mainExchange},
		{n.dlq, n.dlq, n.dlxExchange},
	} {
		if err := t.ch.QueueBind(b.queue, b.key, b.exchange, false, nil); err != nil {
			return fmt.Errorf("rabbitmq: bind %s: %w", b.queue, err)
		}
	}

	return nil
}

func (t *Topology) DeclareFanout(cfg FanoutTopologyConfig) error {
	fanoutExchange := cfg.EventName + ".exchange"
	retryExchange := cfg.EventName + ".retry.exchange"
	dlxExchange := cfg.EventName + ".dlx"

	if err := t.declareExchange(fanoutExchange, "fanout"); err != nil {
		return err
	}
	if err := t.declareExchange(retryExchange, "direct"); err != nil {
		return err
	}
	if err := t.declareExchange(dlxExchange, "direct"); err != nil {
		return err
	}

	for _, queueBase := range cfg.Queues {
		n := buildNames(cfg.EventName, queueBase)

		if err := t.declareQueueWithDLX(n.mainQueue, retryExchange, n.retryQueue); err != nil {
			return err
		}
		if err := t.declareRetryQueue(n.retryQueue, retryExchange, n.mainQueue, cfg.RetryTTL); err != nil {
			return err
		}
		if err := t.declareDLQ(n.dlq); err != nil {
			return err
		}
		if err := t.ch.QueueBind(n.mainQueue, "", fanoutExchange, false, nil); err != nil {
			return fmt.Errorf("rabbitmq: bind fanout %s: %w", n.mainQueue, err)
		}
		if err := t.ch.QueueBind(n.retryQueue, n.retryQueue, retryExchange, false, nil); err != nil {
			return fmt.Errorf("rabbitmq: bind retry %s: %w", n.retryQueue, err)
		}
		if err := t.ch.QueueBind(n.dlq, n.dlq, dlxExchange, false, nil); err != nil {
			return fmt.Errorf("rabbitmq: bind dlq %s: %w", n.dlq, err)
		}
	}

	return nil
}

func (t *Topology) DeclareTopic(cfg TopicTopologyConfig) error {
	mainExchange := cfg.EventName + ".exchange"
	dlxExchange := cfg.EventName + ".dlx"

	if err := t.declareExchange(mainExchange, "topic"); err != nil {
		return err
	}
	if err := t.declareExchange(dlxExchange, "direct"); err != nil {
		return err
	}

	declaredQueues := map[string]struct{}{}

	for _, b := range cfg.Bindings {
		if _, already := declaredQueues[b.Queue]; !already {
			n := buildNames(cfg.EventName, b.Queue)

			if err := t.declareQueueWithDLX(n.mainQueue, mainExchange, n.retryQueue); err != nil {
				return err
			}
			if err := t.declareRetryQueue(n.retryQueue, mainExchange, n.mainQueue, cfg.RetryTTL); err != nil {
				return err
			}
			if err := t.declareDLQ(n.dlq); err != nil {
				return err
			}
			if err := t.ch.QueueBind(n.retryQueue, n.retryQueue, mainExchange, false, nil); err != nil {
				return fmt.Errorf("rabbitmq: bind retry %s: %w", n.retryQueue, err)
			}
			if err := t.ch.QueueBind(n.dlq, n.dlq, dlxExchange, false, nil); err != nil {
				return fmt.Errorf("rabbitmq: bind dlq %s: %w", n.dlq, err)
			}

			declaredQueues[b.Queue] = struct{}{}
		}

		n := buildNames(cfg.EventName, b.Queue)
		if err := t.ch.QueueBind(n.mainQueue, b.RoutingKey, mainExchange, false, nil); err != nil {
			return fmt.Errorf("rabbitmq: bind topic %s->%s: %w", b.RoutingKey, n.mainQueue, err)
		}
	}

	return nil
}

func (t *Topology) declareExchange(name, kind string) error {
	return t.ch.ExchangeDeclare(name, kind, true, false, false, false, nil)
}

func (t *Topology) declareQueueWithDLX(queue, dlxExchange, dlxRoutingKey string) error {
	_, err := t.ch.QueueDeclare(
		queue, true, false, false, false,
		amqp.Table{
			"x-dead-letter-exchange":    dlxExchange,
			"x-dead-letter-routing-key": dlxRoutingKey,
		},
	)
	return err
}

func (t *Topology) declareRetryQueue(queue, dlxExchange, dlxRoutingKey string, ttl time.Duration) error {
	_, err := t.ch.QueueDeclare(
		queue, true, false, false, false,
		amqp.Table{
			"x-message-ttl":             ttl.Milliseconds(),
			"x-dead-letter-exchange":    dlxExchange,
			"x-dead-letter-routing-key": dlxRoutingKey,
		},
	)
	return err
}

func (t *Topology) declareDLQ(queue string) error {
	_, err := t.ch.QueueDeclare(queue, true, false, false, false, nil)
	return err
}
