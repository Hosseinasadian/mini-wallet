package event

import (
	"context"
	"github.com/hosseinasadian/mini-wallet/pkg/logger"
	"github.com/hosseinasadian/mini-wallet/pkg/richerror"
)

type Repository interface {
	RunProcess(ctx context.Context, messageId string, payload []byte) (err error)
	MarkSent(ctx context.Context, messageId string) error
	MarkFailed(ctx context.Context, messageId string) error
}

type Service struct {
	repo   Repository
	logger *logger.Logger
}

func NewService(repo Repository, logger *logger.Logger) *Service {
	return &Service{
		repo:   repo,
		logger: logger,
	}
}

func (s *Service) RunProcess(ctx context.Context, messageId string, payload []byte) error {
	const op = "event.RunProcess"

	if err := s.repo.RunProcess(ctx, messageId, payload); err != nil {
		return richerror.New(op).WithWrapper(err)
	}

	return nil
}

func (s *Service) MarkSent(ctx context.Context, messageId string) error {
	const op = "event.MarkSent"

	if err := s.repo.MarkSent(ctx, messageId); err != nil {
		return richerror.New(op).
			WithWrapper(err).
			WithKind(richerror.KindInternal).
			WithMessage("failed to mark message as sent")
	}

	return nil
}

func (s *Service) MarkFailed(ctx context.Context, messageId string) error {
	const op = "event.MarkFailed"

	if err := s.repo.MarkFailed(ctx, messageId); err != nil {
		return richerror.New(op).
			WithWrapper(err).
			WithKind(richerror.KindInternal).
			WithMessage("failed to mark message as failed")
	}

	return nil
}
