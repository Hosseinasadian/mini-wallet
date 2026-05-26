package notification

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"github.com/hosseinasadian/mini-wallet/pkg/logger"
	"github.com/hosseinasadian/mini-wallet/pkg/redis"
	"net/http"
	"strconv"
	"time"
)

const ticketPrefix = "ticket:"

var ErrTicketNotFound = errors.New("ticket not found or expired")

type Config struct {
	TicketTTL time.Duration `koanf:"ticket_ttl"`
}

type Service struct {
	config Config
	redis  *redis.Redis
	logger *logger.Logger
}

func NewService(config Config, redis *redis.Redis, logger *logger.Logger) *Service {
	return &Service{
		config: config,
		redis:  redis,
		logger: logger,
	}
}

func (s *Service) IsReady(ctx context.Context) (error, int) {
	rErr := s.redis.Ping(ctx)
	if rErr != nil {
		return errors.New("redis down"), http.StatusServiceUnavailable
	}

	return nil, http.StatusOK
}

func (s *Service) CreateTicket(ctx context.Context, userId int64) (string, error) {
	ticket, tErr := generateTicketID()
	if tErr != nil {
		s.logger.Error("failed to generate ticket", "error", tErr)
		return "", tErr
	}

	err := s.redis.Client().Set(ctx, ticketPrefix+ticket, userId, s.config.TicketTTL).Err()
	if err != nil {
		s.logger.Error("failed to save ticket", "error", err)
		return "", fmt.Errorf("internal Server Error")
	}

	return ticket, nil
}

func (s *Service) ConsumeTicket(ctx context.Context, code string) (int64, error) {
	ticket := ticketPrefix + code
	idString, err := s.redis.Client().GetDel(ctx, ticket).Result()

	if redis.IsNil(err) {
		return 0, ErrTicketNotFound
	}

	if err != nil {
		s.logger.Error("failed to get ticket", "error", err)
		return 0, fmt.Errorf("internal Server Error")
	}

	userId, err := strconv.ParseInt(idString, 10, 64)
	if err != nil {
		s.logger.Error("failed to parse ticket id", "error", err)
		return 0, fmt.Errorf("internal Server Error")
	}

	return userId, nil
}

func generateTicketID() (string, error) {
	b := make([]byte, 16) // 32 chars hex
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}
