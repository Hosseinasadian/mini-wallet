package device

import (
	"context"
	"errors"
	"fmt"
	"github.com/hosseinasadian/mini-wallet/pkg/redis"
	"github.com/hosseinasadian/mini-wallet/pkg/sender"
	"log"
	"time"
)

const (
	CacheKeyOneSignal = "one_signal_token"
	CacheKeyFirebase  = "firebase_token"
	CachedUserId      = "user_id"
)

type Details struct {
	OneSignalToken *string `json:"onesignal_token,omitempty"`
	FirebaseToken  *string `json:"firebase_token,omitempty"`
	UserId         *string `json:"user_id,omitempty"`
}

type Device struct {
	DeviceId string `json:"device_id"`
	Details
}

type Config struct {
	CacheTTL time.Duration `json:"cache_ttl"`
}

type Service struct {
	redisAdapter *redis.Redis
	repo         sender.Repository
	config       Config
}

func deviceKey(deviceID string) string {
	return fmt.Sprintf("notif-device:%s", deviceID)
}

func (s *Service) getTokenFromRedis(ctx context.Context, deviceID string, providerCacheKey string) (string, error) {
	key := deviceKey(deviceID)

	fields, err := s.redisAdapter.Client().HGetAll(ctx, key).Result()
	if err != nil {
		return "", err
	}

	if len(fields) == 0 {
		return "", redis.Nil
	}

	if token, ok := fields[providerCacheKey]; ok && token != "" {
		return token, nil
	}

	return "", nil
}

func (s *Service) saveTokenToRedis(ctx context.Context, deviceID string, token string, providerCacheKey string) error {
	key := deviceKey(deviceID)

	err := s.redisAdapter.Client().HSet(ctx, key, map[string]interface{}{
		providerCacheKey: token,
	}).Err()

	if err != nil {
		return err
	}

	return s.redisAdapter.Client().Expire(ctx, key, s.config.CacheTTL).Err()
}

func (s *Service) GetOneSignalToken(ctx context.Context, deviceId string) (string, error) {
	token, err := s.getTokenFromRedis(ctx, deviceId, CacheKeyOneSignal)
	if err == nil && token != "" {
		log.Printf("Service %s found in Redis", deviceId)
		return token, nil
	}

	if !errors.Is(err, redis.Nil) {
		log.Printf("Redis error: %v, falling back to DB", err)
	}

	token, err = s.repo.GetOneSignalToken(ctx, deviceId)
	if err != nil {
		return "", fmt.Errorf("device not found in any storage: %w", err)
	}

	err = s.saveTokenToRedis(ctx, deviceId, token, CacheKeyOneSignal)
	if err != nil {
		log.Printf("Failed to restore device %s to Redis: %v", deviceId, err)
	}

	return token, nil
}

func (s *Service) GetFirebaseToken(ctx context.Context, deviceId string) (string, error) {
	token, err := s.getTokenFromRedis(ctx, deviceId, CacheKeyFirebase)
	if err == nil && token != "" {
		log.Printf("Service %s found in Redis", deviceId)
		return token, nil
	}

	if !errors.Is(err, redis.Nil) {
		log.Printf("Redis error: %v, falling back to DB", err)
	}

	token, err = s.repo.GetFirebaseToken(ctx, deviceId)
	if err != nil {
		return "", fmt.Errorf("device not found in any storage: %w", err)
	}

	err = s.saveTokenToRedis(ctx, deviceId, token, CacheKeyFirebase)
	if err != nil {
		log.Printf("Failed to restore device %s to Redis: %v", deviceId, err)
	}

	return token, nil
}

func (s *Service) LinkDeviceToUser(ctx context.Context, deviceId, userID string) error {
	err := s.repo.LinkDeviceToUser(ctx, deviceId, userID)
	if err != nil {
		return err
	}

	err = s.redisAdapter.Client().Del(ctx, deviceKey(deviceId)).Err()
	if err != nil {
		log.Printf("Warning: Failed to invalidate cache for device %s: %v", deviceId, err)
	}

	return nil
}

func (s *Service) UnlinkDeviceFromUser(ctx context.Context, deviceID string) error {
	err := s.repo.UnlinkDeviceFromUser(ctx, deviceID)
	if err != nil {
		return err
	}

	err = s.redisAdapter.Client().Del(ctx, deviceKey(deviceID)).Err()
	if err != nil {
		log.Printf("Warning: Failed to invalidate cache for device %s: %v", deviceID, err)
	}

	return nil
}
