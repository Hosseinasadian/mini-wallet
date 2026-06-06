package subscriber

import (
	"context"
	authpb "github.com/hosseinasadian/mini-wallet/gen/go/auth/v1"
	commonpb "github.com/hosseinasadian/mini-wallet/gen/go/common/v1"
	"github.com/hosseinasadian/mini-wallet/pkg/broker"
	"github.com/hosseinasadian/mini-wallet/pkg/event"
	pkgLogger "github.com/hosseinasadian/mini-wallet/pkg/logger"
	"google.golang.org/protobuf/proto"
)

type Handler struct {
	logger *pkgLogger.Logger
}

func New(logger *pkgLogger.Logger) *Handler {
	return &Handler{
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
	case string(event.TypeAuthRegisterNewUser):
		var registeredEv authpb.RegisterNewUser
		err := ev.Payload.UnmarshalTo(&registeredEv)
		if err != nil {
			h.logger.Error("failed to unmarshal register event", "error", err)
			return err
		} else {
			h.logger.Info("registered new user", "email", registeredEv.Email)
			return nil
		}
	}

	return nil
}
