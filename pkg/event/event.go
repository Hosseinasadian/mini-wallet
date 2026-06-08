package event

import (
	commonpb "github.com/hosseinasadian/mini-wallet/gen/go/common/v1"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"
)

type Type string

const (
	TypeAuthNewSessionLoggedIn Type = "auth.v1.NewSessionLoggedIn"
	TypeAuthRegisterNewUser    Type = "auth.v1.RegisterNewUser"
	TypeNotificationPush       Type = "notification.v1.push"
)

func New(ev proto.Message, eventType Type) (*commonpb.EventEnvelope, error) {
	newLoggedEventPayload, err := anypb.New(ev)
	if err != nil {
		return nil, err
	}

	return &commonpb.EventEnvelope{
		EventType: string(eventType),
		Payload:   newLoggedEventPayload,
	}, nil
}
