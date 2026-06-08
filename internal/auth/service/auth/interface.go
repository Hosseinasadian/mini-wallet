package auth

import (
	"context"
	outboxService "github.com/hosseinasadian/mini-wallet/internal/auth/service/outbox"
	"time"
)

type Repository interface {
	InsertOutboxEvent(ctx context.Context, event *outboxService.OutboxEvent) error
	CreateUserByEmailAndPassword(ctx context.Context, email, password string) (int64, error)
	UpsertSession(ctx context.Context, deviceCtx *DeviceContext, userID int64, refreshTokenHash string, expiresAt time.Time, pushToken *string) (string, string, error)
}

type RepositoryTx interface {
	RunInTx(ctx context.Context, fn func(exec Repository) error) error
	Ping(ctx context.Context) error

	GetUserByEmail(ctx context.Context, email string) (*User, error)
	RotateRefreshToken(ctx context.Context, deviceCtx *DeviceContext, oldRefreshTokenHash string, newRefreshTokenHash string, newExpiresAt time.Time, pushToken *string) (string, string, int64, error)
	GetUserSessions(ctx context.Context, userID int64) ([]SessionItem, error)
	RevokeSession(ctx context.Context, userID int64, sessionPublicID string, reason string, revokedBy string) error
	RevokeAllSessions(ctx context.Context, userID int64, exceptSessionID *string, reason string, revokedBy string) error
	UpdatePushToken(ctx context.Context, sessionID string, userID int64, pushToken string) error
}
