package outbox

import (
	"context"
	"github.com/hosseinasadian/mini-wallet/pkg/broker"
	"github.com/hosseinasadian/mini-wallet/pkg/event"
	pkgLogger "github.com/hosseinasadian/mini-wallet/pkg/logger"
	"github.com/hosseinasadian/mini-wallet/pkg/richerror"
	"time"
)

type Repository interface {
	ClaimOutboxEvents(ctx context.Context, count int64, lockTimeout time.Duration) ([]OutboxEvent, error)
	ClaimSingleEvent(ctx context.Context, eventId string, lockTimeout time.Duration) error
	MarkOutboxEventProcessed(ctx context.Context, eventId string) error
	MarkOutboxEventFailed(ctx context.Context, eventId string) error
	MarkOutboxEventsProcessed(ctx context.Context, eventIds []string) error
	MarkOutboxEventsFailed(ctx context.Context, eventIds []string) error
	RecoverStaleOutboxEvents(ctx context.Context) (int64, error)
}

type Config struct {
	LockTimeout time.Duration `koanf:"lock_timeout"`
}

type Service struct {
	repo                  Repository
	config                Config
	logger                *pkgLogger.Logger
	userPublisher         broker.TopicPublisher
	notificationPublisher broker.DirectPublisher
}

func NewService(repo Repository, config Config, userPublisher broker.TopicPublisher, notificationPublisher broker.DirectPublisher, logger *pkgLogger.Logger) *Service {
	return &Service{
		repo:                  repo,
		config:                config,
		userPublisher:         userPublisher,
		notificationPublisher: notificationPublisher,
		logger:                logger,
	}
}

func (s *Service) ClaimOutboxEvents(ctx context.Context, count int64, lockTimeout time.Duration) ([]OutboxEvent, error) {
	const op = "worker.ClaimOutboxEvents"

	events, err := s.repo.ClaimOutboxEvents(ctx, count, lockTimeout)
	if err != nil {
		return nil, richerror.New(op).
			WithWrapper(err).
			WithKind(richerror.KindInternal).
			WithMessage("failed to list outbox events")
	}

	return events, nil
}

func (s *Service) ProcessEvent(ctx context.Context, e *OutboxEvent) error {

	switch e.EventType {
	case string(event.TypeAuthNewSessionLoggedIn):
		return s.notificationPublisher.Publish(ctx, e.Payload)
	case string(event.TypeAuthRegisterNewUser):
		return s.userPublisher.Publish(ctx, "user.created", e.Payload)
	default:
		return nil
	}
}

func (s *Service) ProcessSingleEvent(ctx context.Context, e *OutboxEvent) error {
	const op = "outbox.ProcessSingleEvent"

	if err := s.repo.ClaimSingleEvent(ctx, e.EventID, s.config.LockTimeout); err != nil {
		return richerror.New(op).
			WithWrapper(err).
			WithKind(richerror.KindInternal).
			WithMessage("failed to claim single event")
	}

	err := s.ProcessEvent(ctx, e)
	if err != nil {
		_ = s.repo.MarkOutboxEventFailed(ctx, e.EventID)
		return richerror.New(op).
			WithWrapper(err).
			WithKind(richerror.KindInternal).
			WithMessage("failed to process event")
	}

	if err = s.repo.MarkOutboxEventProcessed(ctx, e.EventID); err != nil {
		s.logger.Warn("failed to mark event as processed", "eventId", e.EventID, "err", err)
	}

	return nil
}

func (s *Service) RecoverStaleEvents(ctx context.Context) (int64, error) {
	const op = "outbox.RecoverStaleEvents"

	count, err := s.repo.RecoverStaleOutboxEvents(ctx)
	if err != nil {
		return 0, richerror.New(op).
			WithWrapper(err).
			WithKind(richerror.KindInternal).
			WithMessage("failed to recover stale events")
	}

	return count, nil
}

func (s *Service) MarkEventsProcessed(ctx context.Context, eventIds []string) error {
	const op = "outbox.MarkEventsProcessed"

	if len(eventIds) == 0 {
		return nil
	}

	if err := s.repo.MarkOutboxEventsProcessed(ctx, eventIds); err != nil {
		return richerror.New(op).
			WithWrapper(err).
			WithKind(richerror.KindInternal).
			WithMessage("failed to mark events as processed")
	}

	return nil
}

func (s *Service) MarkEventsFailed(ctx context.Context, eventIds []string) error {
	const op = "outbox.MarkEventsFailed"

	if len(eventIds) == 0 {
		return nil
	}

	if err := s.repo.MarkOutboxEventsFailed(ctx, eventIds); err != nil {
		return richerror.New(op).
			WithWrapper(err).
			WithKind(richerror.KindInternal).
			WithMessage("failed to mark events as failed")
	}

	return nil
}
