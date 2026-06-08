package subscriber

import (
	"context"
	"errors"
	authpb "github.com/hosseinasadian/mini-wallet/gen/go/auth/v1"
	commonpb "github.com/hosseinasadian/mini-wallet/gen/go/common/v1"
	eventService "github.com/hosseinasadian/mini-wallet/internal/wallet/service/event"
	walletService "github.com/hosseinasadian/mini-wallet/internal/wallet/service/wallet"
	"github.com/hosseinasadian/mini-wallet/pkg/broker"
	"github.com/hosseinasadian/mini-wallet/pkg/event"
	pkgLogger "github.com/hosseinasadian/mini-wallet/pkg/logger"
	"github.com/hosseinasadian/mini-wallet/pkg/richerror"
	"google.golang.org/protobuf/proto"
)

type Handler struct {
	walletSvc *walletService.Service
	eventSvc  *eventService.Service
	logger    *pkgLogger.Logger
}

func New(walletSvc *walletService.Service, eventSvc *eventService.Service, logger *pkgLogger.Logger) *Handler {
	return &Handler{
		walletSvc: walletSvc,
		eventSvc:  eventSvc,
		logger:    logger,
	}
}

func (h *Handler) Handle(ctx context.Context, msg broker.Message) error {
	err := h.eventSvc.RunProcess(ctx, msg.ID, msg.Body)
	if err != nil {
		var re *richerror.RichError
		if errors.As(err, &re) && re.Kind() == richerror.KindConflict {
			h.logger.Info("duplicate message skipped", "msg_id", msg.ID)
			return nil
		}
		return err
	}

	var ev commonpb.EventEnvelope

	if err := proto.Unmarshal(msg.Body, &ev); err != nil {
		h.logger.Error("failed to unmarshal message", "error", err)
		return err
	}

	if err := h.handleEvent(ctx, &ev); err != nil {
		err = h.eventSvc.MarkFailed(ctx, msg.ID)
		if err != nil {
			h.logger.Warn("failed to mark failed event", "error", err, "msg_id", msg.ID)
		}
		return err
	}

	err = h.eventSvc.MarkSent(ctx, msg.ID)
	if err != nil {
		h.logger.Warn("failed to mark sent event", "error", err, "msg_id", msg.ID)
	}

	return nil
}

func (h *Handler) handleEvent(ctx context.Context, ev *commonpb.EventEnvelope) error {
	switch ev.EventType {
	case string(event.TypeAuthRegisterNewUser):
		return h.handleRegisterNewUser(ctx, ev)
	default:
		return nil
	}
}

func (h *Handler) handleRegisterNewUser(ctx context.Context, ev *commonpb.EventEnvelope) error {
	var registeredEv authpb.RegisterNewUser
	err := ev.Payload.UnmarshalTo(&registeredEv)
	if err != nil {
		h.logger.Error("failed to unmarshal register event", "error", err)
		return err
	}

	walletId, err := h.walletSvc.InitWallet(ctx, uint64(registeredEv.Id))
	if err != nil {
		var re *richerror.RichError
		if errors.As(err, &re) && re.Kind() == richerror.KindConflict {
			h.logger.Info("duplicate wallet skipped", "user_id", registeredEv.Id)
			return nil
		}

		h.logger.Error("failed to init wallet", "user_id", registeredEv.Id, "error", err)
		return err
	}

	h.logger.Info("init wallet for new user", "uuid", walletId)
	return nil

}
