package subscriber

import (
	"context"
	"fmt"
	authpb "github.com/hosseinasadian/mini-wallet/gen/go/auth/v1"
	commonpb "github.com/hosseinasadian/mini-wallet/gen/go/common/v1"
	"github.com/hosseinasadian/mini-wallet/pkg/broker"
	"github.com/hosseinasadian/mini-wallet/pkg/event"
	"github.com/hosseinasadian/mini-wallet/pkg/hub"
	pkgLogger "github.com/hosseinasadian/mini-wallet/pkg/logger"
	"google.golang.org/protobuf/proto"
)

type Sender interface {
	Send(ctx context.Context, userID string, title, message string) error
}

type Handler struct {
	hub    *hub.Hub
	sender Sender
	logger *pkgLogger.Logger
}

func New(hub *hub.Hub, sender Sender, logger *pkgLogger.Logger) *Handler {
	return &Handler{
		hub:    hub,
		sender: sender,
		logger: logger,
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
			userIdStr := fmt.Sprintf("%d", newSessionEv.UserId)
			msg := fmt.Sprintf("user %s logged in with session %s", userIdStr, newSessionEv.Id)
			if h.hub.IsOnline(userIdStr) {
				h.hub.Publish(userIdStr, msg)
			} else {
				if err := h.sender.Send(ctx, userIdStr, "new auth", msg); err != nil {
					h.logger.Error("send auth failed", "error", err)
				}
			}
			return nil
		}
	}

	return nil
}
