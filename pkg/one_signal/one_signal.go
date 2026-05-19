package one_signal

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

type Config struct {
	AppID  string `koanf:"app_id"`
	ApiKey string `koanf:"api_key"`
}

type FakeRepo struct {
}

func (repo *FakeRepo) GetOneSignalToken(ctx context.Context, deviceId string) (string, error) {
	return "", nil
}

type Repository interface {
	GetOneSignalToken(ctx context.Context, deviceId string) (string, error)
}

type Sender struct {
	config Config
	repo   Repository
}

func NewOneSignalSender(config Config, repository Repository) *Sender {
	return &Sender{config: config, repo: repository}
}

func (s *Sender) Send(ctx context.Context, deviceId string, title, message string) error {
	token, err := s.repo.GetOneSignalToken(ctx, deviceId)
	if err != nil {
		return err
	}

	payload := map[string]any{
		"app_id": s.config.AppID,
		"headings": map[string]string{
			"en": title,
		},
		"contents": map[string]string{
			"en": message,
		},
		"target_channel": "push",
		"include_aliases": map[string][]string{
			"external_id": {token},
		},
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(
		ctx,
		"POST",
		"https://api.onesignal.com/notifications?c=push",
		bytes.NewBuffer(body),
	)
	if err != nil {
		return err
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Key "+s.config.ApiKey)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("onesignal error: %d - %s", resp.StatusCode, string(b))
	}

	return nil
}
