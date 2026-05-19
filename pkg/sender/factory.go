package sender

import (
	"context"
	"fmt"
	"github.com/hosseinasadian/mini-wallet/pkg/firebase"
	"github.com/hosseinasadian/mini-wallet/pkg/one_signal"
)

type Config struct {
	Provider  ProviderType      `koanf:"provider"`
	OneSignal one_signal.Config `koanf:"one_signal"`
	Firebase  firebase.Config   `koanf:"firebase"`
}

type Factory struct {
	config Config
}

func NewFactory(config Config) (*Factory, error) {
	if config.Provider == "" {
		return nil, fmt.Errorf("provider type is required")
	}

	switch config.Provider {
	case ProviderOneSignal:
		if config.OneSignal.AppID == "" || config.OneSignal.ApiKey == "" {
			return nil, fmt.Errorf("one_signal config is incomplete")
		}
	case ProviderFirebase:
		if config.Firebase.CredentialsFile == "" {
			return nil, fmt.Errorf("firebase config is incomplete")
		}
	default:
		return nil, fmt.Errorf("unsupported provider: %s", config.Provider)
	}

	return &Factory{config: config}, nil
}

func (f *Factory) CreateSender(ctx context.Context, repo Repository) (Sender, error) {
	switch f.config.Provider {
	case ProviderOneSignal:
		return one_signal.NewOneSignalSender(f.config.OneSignal, repo), nil
	case ProviderFirebase:
		return firebase.NewFirebaseSender(ctx, f.config.Firebase, repo)
	default:
		return nil, fmt.Errorf("unsupported provider: %s", f.config.Provider)
	}
}
