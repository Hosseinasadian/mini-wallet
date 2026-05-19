package broker

import (
	"context"
	"time"
)

type TestCommit struct {
}

type Message struct {
	ID        string
	Body      []byte
	Headers   map[string]any
	Timestamp time.Time
}

type Handler func(ctx context.Context, msg Message) error

type DirectPublisher interface {
	Publish(ctx context.Context, body []byte) error
	Close() error
}

type FanoutPublisher interface {
	Publish(ctx context.Context, body []byte) error
	Close() error
}

type TopicPublisher interface {
	Publish(ctx context.Context, routingKey string, body []byte) error
	Close() error
}

type Subscriber interface {
	Subscribe(handler Handler) error
	Close(ctx context.Context) error
}

type Config struct {
	URL string `koanf:"url"`

	RetryTTL time.Duration `koanf:"retry_ttl"`

	Workers        int           `koanf:"workers"`
	MaxRetry       int64         `koanf:"max_retry"`
	PrefetchCount  int           `koanf:"prefetch_count"`
	HandlerTimeout time.Duration `koanf:"handler_timeout"`
}
