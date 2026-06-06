package subscriber

import (
	"context"
	"fmt"
	authpb "github.com/hosseinasadian/mini-wallet/gen/go/auth/v1"
	commonpb "github.com/hosseinasadian/mini-wallet/gen/go/common/v1"
	"github.com/hosseinasadian/mini-wallet/internal/auth/service/auth"
	"github.com/hosseinasadian/mini-wallet/pkg/broker"
	"github.com/hosseinasadian/mini-wallet/pkg/event"
	pkgLogger "github.com/hosseinasadian/mini-wallet/pkg/logger"
	"google.golang.org/protobuf/proto"
)

type Handler struct {
	logger  *pkgLogger.Logger
	authSvc *auth.Service
}

func New(logger *pkgLogger.Logger, authSvc *auth.Service) *Handler {
	return &Handler{
		logger:  logger,
		authSvc: authSvc,
	}
}

func (h *Handler) Handle(ctx context.Context, msg broker.Message) error {
	var ev commonpb.EventEnvelope

	if err := proto.Unmarshal(msg.Body, &ev); err != nil {
		h.logger.Error("failed to unmarshal message", "error", err)
		return err
	}

	switch ev.EventType {
	case string(event.TypeAuthNewSessionLoggedIn):
		var newSessionEv authpb.NewSessionLoggedIn
		err := ev.Payload.UnmarshalTo(&newSessionEv)
		if err != nil {
			h.logger.Error("failed to unmarshal login event", "error", err)
			return err
		} else {
			h.logger.Info("registered new user", "session", newSessionEv.Id, "userId", newSessionEv.UserId)
			sessions, err := h.authSvc.GetActiveSessions(ctx, newSessionEv.Id)
			if err != nil {
				h.logger.Error("failed to get active sessions", "error", err)
				return err
			}

			return nil
		}
	}

	return nil
}
