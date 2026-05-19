package rabbitmq

import (
	"errors"
	"fmt"

	amqp "github.com/rabbitmq/amqp091-go"
)

type Connection struct {
	conn *amqp.Connection
}

func NewConnection(url string) (*Connection, error) {
	conn, err := amqp.Dial(url)
	if err != nil {
		return nil, fmt.Errorf("rabbitmq: dial: %w", err)
	}
	return &Connection{conn: conn}, nil
}

func (c *Connection) Close() error {
	if err := c.conn.Close(); err != nil {
		return fmt.Errorf("rabbitmq: close connection: %w", err)
	}
	return nil
}

func (c *Connection) channel() (*amqp.Channel, error) {
	ch, err := c.conn.Channel()
	if err != nil {
		return nil, fmt.Errorf("rabbitmq: open channel: %w", err)
	}
	return ch, nil
}

var ErrClosed = errors.New("rabbitmq: closed")
