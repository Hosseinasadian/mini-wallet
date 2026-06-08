package rabbitmq

import (
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
)

type Connection struct {
	url    string
	conn   *amqp.Connection
	mu     sync.RWMutex
	closed atomic.Bool
}

func NewConnection(url string) (*Connection, error) {
	c := &Connection{url: url}
	if err := c.connect(); err != nil {
		return nil, err
	}
	go c.watchReconnect()
	return c, nil
}

func (c *Connection) connect() error {
	conn, err := amqp.Dial(c.url)
	if err != nil {
		return fmt.Errorf("rabbitmq: dial: %w", err)
	}
	c.mu.Lock()
	c.conn = conn
	c.mu.Unlock()
	return nil
}

func (c *Connection) watchReconnect() {
	for {
		if c.closed.Load() {
			return
		}

		c.mu.RLock()
		conn := c.conn
		c.mu.RUnlock()

		reason := <-conn.NotifyClose(make(chan *amqp.Error, 1))
		if reason == nil || c.closed.Load() {
			return
		}

		for {
			if c.closed.Load() {
				return
			}

			time.Sleep(3 * time.Second)

			if err := c.connect(); err != nil {
				continue
			}

			break
		}
	}
}

func (c *Connection) Close() error {
	if err := c.conn.Close(); err != nil {
		return fmt.Errorf("rabbitmq: close connection: %w", err)
	}
	return nil
}

func (c *Connection) channel() (*amqp.Channel, error) {
	c.mu.RLock()
	conn := c.conn
	c.mu.RUnlock()

	ch, err := conn.Channel()
	if err != nil {
		return nil, fmt.Errorf("rabbitmq: open channel: %w", err)
	}
	return ch, nil
}

var ErrClosed = errors.New("rabbitmq: closed")
