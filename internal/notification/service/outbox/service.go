package outbox

import (
	"context"
	commonpb "github.com/hosseinasadian/mini-wallet/gen/go/common/v1"
	notificationpb "github.com/hosseinasadian/mini-wallet/gen/go/notification/v1"
	"github.com/hosseinasadian/mini-wallet/pkg/broker"
	"github.com/hosseinasadian/mini-wallet/pkg/event"
	pkgLogger "github.com/hosseinasadian/mini-wallet/pkg/logger"
	"github.com/hosseinasadian/mini-wallet/pkg/richerror"
	"google.golang.org/protobuf/proto"
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

	InsertOutboxEvent(ctx context.Context, event *OutboxEvent) error
	InsertOutboxEvents(ctx context.Context, events []*OutboxEvent) error
}

type Config struct {
	LockTimeout time.Duration `koanf:"lock_timeout"`
}

type Service struct {
	repo      Repository
	config    Config
	logger    *pkgLogger.Logger
	publisher broker.DirectPublisher
}

func NewService(repo Repository, config Config, publisher broker.DirectPublisher, logger *pkgLogger.Logger) *Service {
	return &Service{
		repo:      repo,
		config:    config,
		publisher: publisher,
		logger:    logger,
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

func (s *Service) ProcessEvent(ctx context.Context, outboxEvent *OutboxEvent) error {

	switch outboxEvent.EventType {
	case string(event.TypeNotificationPush):
		var ev commonpb.EventEnvelope

		if err := proto.Unmarshal(outboxEvent.Payload, &ev); err != nil {
			s.logger.Error("failed to unmarshal message", "error", err)
			return err
		}

		var notifEv notificationpb.NotificationEvent
		err := ev.Payload.UnmarshalTo(&notifEv)
		if err != nil {
			s.logger.Error("failed to unmarshal login event", "error", err)
			return err
		} else {
			s.logger.Info("login new session", "session", notifEv.EventId)

			return s.publisher.Publish(ctx, outboxEvent.Payload)
		}

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

func (s *Service) InsertOutboxEvent(ctx context.Context, event *OutboxEvent) error {
	const op = "outbox.InsertOutboxEvent"
	if err := s.repo.InsertOutboxEvent(ctx, event); err != nil {
		return richerror.New(op).
			WithWrapper(err).
			WithKind(richerror.KindInternal).
			WithMessage("failed to insert outbox event")
	}

	return nil
}

func (s *Service) InsertOutboxEvents(ctx context.Context, events []*OutboxEvent) error {
	const op = "outbox.InsertOutboxEvents"
	if err := s.repo.InsertOutboxEvents(ctx, events); err != nil {
		return richerror.New(op).
			WithWrapper(err).
			WithKind(richerror.KindInternal).
			WithMessage("failed to insert outbox events")
	}

	return nil
}
