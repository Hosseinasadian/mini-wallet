package redis

import (
	"context"
	"errors"
	"fmt"
	"github.com/redis/go-redis/v9"
)

type Config struct {
	Host     string `koanf:"host"`
	Port     int    `koanf:"port"`
	Password string `koanf:"password"`
	DB       int    `koanf:"db"`
}
type Redis struct {
	client *redis.Client
}

const Nil = redis.Nil

func New(ctx context.Context, config Config) (*Redis, error) {
	if ctx == nil {
		return nil, fmt.Errorf("context cannot be nil")
	}

	addr := fmt.Sprintf("%s:%d", config.Host, config.Port)

	rdb := redis.NewClient(&redis.Options{
		Addr:     addr,
		Password: config.Password,
		DB:       config.DB,
	})

	if err := rdb.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("failed to connect to redis: %w", err)
	}

	return &Redis{client: rdb}, nil
}

func (a *Redis) Client() *redis.Client {
	return a.client
}

func (a *Redis) Close() error {
	if a == nil || a.client == nil {
		return nil
	}

	err := a.client.Close()
	if err != nil {
		return fmt.Errorf("failed to close redis: %w", err)
	}

	return nil
}

func IsNil(err error) bool {
	return errors.Is(err, redis.Nil)
}
