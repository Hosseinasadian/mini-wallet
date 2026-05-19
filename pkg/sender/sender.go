package sender

import (
	"context"
	"github.com/hosseinasadian/mini-wallet/pkg/firebase"
	"github.com/hosseinasadian/mini-wallet/pkg/one_signal"
)

type ProviderType string

const (
	ProviderOneSignal ProviderType = "one_signal"
	ProviderFirebase  ProviderType = "firebase"
)

type Repository interface {
	one_signal.Repository
	firebase.Repository
	LinkDeviceToUser(ctx context.Context, deviceId, userID string) error
	UnlinkDeviceFromUser(ctx context.Context, deviceID string) error
}

type Sender interface {
	Send(ctx context.Context, deviceId string, title, message string) error
}
