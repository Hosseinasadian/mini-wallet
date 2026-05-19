package firebase

import (
	"context"
	"firebase.google.com/go/v4"
	"firebase.google.com/go/v4/messaging"
	"google.golang.org/api/option"
)

type Config struct {
	CredentialsFile string `koanf:"credentials_file"`
}

type Repository interface {
	GetFirebaseToken(ctx context.Context, deviceId string) (string, error)
}

type Sender struct {
	client *messaging.Client
	repo   Repository
}

func NewFirebaseSender(ctx context.Context, config Config, repo Repository) (*Sender, error) {
	app, err := firebase.NewApp(ctx, nil,
		option.WithAuthCredentialsFile(option.ServiceAccount, config.CredentialsFile),
	)
	if err != nil {
		return nil, err
	}

	client, err := app.Messaging(ctx)
	if err != nil {
		return nil, err
	}

	return &Sender{client: client, repo: repo}, nil
}

func (s *Sender) Send(ctx context.Context, deviceId string, title, body string) error {
	token, err := s.repo.GetFirebaseToken(ctx, deviceId)
	if err != nil {
		return err
	}

	message := &messaging.Message{
		Token: token,
		Notification: &messaging.Notification{
			Title: title,
			Body:  body,
		},
	}

	_, err = s.client.Send(ctx, message)
	return err
}
