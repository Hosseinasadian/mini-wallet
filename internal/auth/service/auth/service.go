package auth

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/go-sql-driver/mysql"
	"github.com/hosseinasadian/mini-wallet/pkg/broker"
	"github.com/hosseinasadian/mini-wallet/pkg/user_access_token"
	"golang.org/x/crypto/bcrypt"
	"log"
	"net/http"
	"regexp"
	"strconv"
	"time"
)

type UserRepository interface {
	CreateUserByEmailAndPassword(ctx context.Context, email, password string) (int64, error)
	GetUserByEmail(ctx context.Context, email string) (*User, error)
	Ping(ctx context.Context) error
}

type TokenRepository interface {
	UpsertSession(ctx context.Context, deviceCtx *DeviceContext, userID int64, refreshTokenHash string, expiresAt time.Time) (string, string, error)
	RotateRefreshToken(ctx context.Context, deviceCtx *DeviceContext, oldRefreshTokenHash string, newRefreshTokenHash string, newExpiresAt time.Time) (string, string, int64, error)
	GetUserSessions(ctx context.Context, userID int64) ([]SessionItem, error)
	RevokeSession(ctx context.Context, userID int64, sessionPublicID string, reason string, revokedBy string) error
	RevokeAllSessions(ctx context.Context, userID int64, exceptSessionID *string, reason string, revokedBy string) error
	Ping(ctx context.Context) error
}

type Config struct {
	JWTSecret            string        `koanf:"jwt_secret"`
	AccessTokenDuration  time.Duration `koanf:"access_token_duration"`
	RefreshTokenDuration time.Duration `koanf:"refresh_token_duration"`
	EmailRegexp          string        `koanf:"email_regexp"`
}

type UserEv struct {
	ID    string `json:"id"`
	Email string `json:"email"`
}

type LoginEv struct {
	Message string `json:"message"`
	UserID  int64  `json:"user_id"`
}

type Service struct {
	userRepo              UserRepository
	tokenRepo             TokenRepository
	config                Config
	userPublisher         broker.TopicPublisher
	notificationPublisher broker.DirectPublisher
}

func NewService(userRepo UserRepository, tokenRepo TokenRepository, config Config, userPublisher broker.TopicPublisher, notificationPublisher broker.DirectPublisher) *Service {
	return &Service{
		userRepo:              userRepo,
		tokenRepo:             tokenRepo,
		config:                config,
		userPublisher:         userPublisher,
		notificationPublisher: notificationPublisher,
	}
}

func (s *Service) IsReady(ctx context.Context) (error, int) {
	urErr := s.userRepo.Ping(ctx)
	if urErr != nil {
		return errors.New("user db down"), http.StatusServiceUnavailable
	}
	trErr := s.tokenRepo.Ping(ctx)
	if trErr != nil {
		return errors.New("token db down"), http.StatusServiceUnavailable
	}

	return nil, http.StatusOK
}

func (s *Service) Register(ctx context.Context, deviceCtx *DeviceContext, req RegisterRequest) (*RegisterResponse, error, int) {
	if len(req.Password) < 8 {
		return nil, fmt.Errorf("password too short"), http.StatusUnprocessableEntity
	}

	if len(req.Password) > 72 {
		return nil, fmt.Errorf("password too long"), http.StatusUnprocessableEntity
	}

	emailRegex := regexp.MustCompile(s.config.EmailRegexp)
	if !emailRegex.MatchString(req.Email) {
		return nil, fmt.Errorf("invalid email"), http.StatusUnprocessableEntity
	}

	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		return nil, fmt.Errorf("failed to process password"), http.StatusInternalServerError
	}

	var accountId int64
	accountId, err = s.userRepo.CreateUserByEmailAndPassword(ctx, req.Email, string(hashedPassword))
	if err != nil {
		var mysqlErr *mysql.MySQLError
		if errors.As(err, &mysqlErr) && mysqlErr.Number == 1062 {
			return nil, fmt.Errorf("email already exists"), http.StatusUnprocessableEntity
		}

		return nil, fmt.Errorf("failed to create user"), http.StatusInternalServerError
	}

	refreshToken, err := generateRefreshToken()
	if err != nil {
		return &RegisterResponse{
			Message: "account created but login failed, please login manually",
		}, nil, http.StatusCreated
	}

	newToken := hashRefreshToken(refreshToken)
	expireRefreshToken := time.Now().Add(s.config.RefreshTokenDuration)
	deviceID, sessionID, err := s.tokenRepo.UpsertSession(
		ctx,
		deviceCtx,
		accountId,
		newToken,
		expireRefreshToken,
	)

	if err != nil {
		return &RegisterResponse{
			Message: "account created but login failed, please login manually",
		}, nil, http.StatusCreated
	}

	var accessToken string
	accessToken, err = user_access_token.GenerateAccessToken(accountId, sessionID, s.config.AccessTokenDuration)
	if err != nil {
		return nil, fmt.Errorf("account created but login failed, please login manually"), http.StatusCreated
	}

	// publish event
	eventBody, mErr := json.Marshal(UserEv{
		ID:    strconv.FormatInt(accountId, 10),
		Email: req.Email,
	})
	if mErr == nil {
		err = s.userPublisher.Publish(
			context.Background(),
			"user.created",
			eventBody,
		)
		if err != nil {
			log.Println(err)
		} else {
			log.Println("Successfully sent event", string(eventBody))
		}
	}

	return &RegisterResponse{
		Message:      "account successfully created",
		AccessToken:  &accessToken,
		RefreshToken: &refreshToken,
		DeviceID:     deviceID,
		SessionID:    sessionID,
	}, nil, http.StatusCreated
}

func (s *Service) Login(ctx context.Context, deviceCtx *DeviceContext, req LoginRequest) (*LoginResponse, error, int) {
	if req.Email == "" {
		return nil, fmt.Errorf("email required"), http.StatusUnprocessableEntity
	}

	if req.Password == "" {
		return nil, fmt.Errorf("password is required"), http.StatusUnprocessableEntity
	}

	user, err := s.userRepo.GetUserByEmail(ctx, req.Email)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("username or password is incorrect"), http.StatusUnauthorized
		}

		return nil, fmt.Errorf("failed to get user"), http.StatusInternalServerError
	}

	err = bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(req.Password))
	if err != nil {
		return nil, fmt.Errorf("username or password is incorrect"), http.StatusUnauthorized
	}

	refreshToken, err := generateRefreshToken()
	if err != nil {
		return nil, fmt.Errorf("failed to generate refresh token"), http.StatusInternalServerError
	}

	expireRefreshToken := time.Now().Add(s.config.RefreshTokenDuration)
	newToken := hashRefreshToken(refreshToken)
	deviceID, sessionID, err := s.tokenRepo.UpsertSession(
		ctx,
		deviceCtx,
		user.ID,
		newToken,
		expireRefreshToken,
	)

	accessToken, err := user_access_token.GenerateAccessToken(user.ID, sessionID, s.config.AccessTokenDuration)
	if err != nil {
		return nil, fmt.Errorf("failed to generate access token"), http.StatusInternalServerError
	}

	// publish event
	lEventBody, lErr := json.Marshal(LoginEv{
		Message: "New user logged in at " + time.Now().Format(time.RFC3339),
		UserID:  user.ID,
	})
	if lErr == nil {
		lErr = s.notificationPublisher.Publish(
			context.Background(),
			lEventBody,
		)
		if lErr != nil {
			log.Println(lErr)
		} else {
			log.Println("Successfully sent event", string(lEventBody))
		}
	}
	/**/

	return &LoginResponse{
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
		DeviceID:     deviceID,
		SessionID:    sessionID,
		User:         user,
	}, nil, http.StatusOK
}

func (s *Service) RefreshToken(ctx context.Context, deviceCtx *DeviceContext, token string) (*RefreshTokenResponse, error, int) {
	newRefreshToken, err := generateRefreshToken()
	if err != nil {
		return nil, fmt.Errorf("failed to generate refresh token"), http.StatusInternalServerError
	}

	expireRefreshToken := time.Now().Add(s.config.RefreshTokenDuration)
	oldToken := hashRefreshToken(token)
	newToken := hashRefreshToken(newRefreshToken)

	deviceID, sessionID, userID, err := s.tokenRepo.RotateRefreshToken(ctx, deviceCtx, oldToken, newToken, expireRefreshToken)

	accessToken, err := user_access_token.GenerateAccessToken(userID, sessionID, s.config.AccessTokenDuration)
	if err != nil {
		return nil, fmt.Errorf("failed to generate access token"), http.StatusInternalServerError
	}

	return &RefreshTokenResponse{
		AccessToken:  accessToken,
		RefreshToken: newRefreshToken,
		DeviceID:     deviceID,
		SessionID:    sessionID,
	}, nil, http.StatusOK
}

func (s *Service) GetUserSessions(ctx context.Context, userID int64) ([]SessionItem, error, int) {

	sessions, err := s.tokenRepo.GetUserSessions(ctx, userID)
	if err != nil {
		log.Printf("failed to get user sessions: %v", err)
		return nil, fmt.Errorf("failed to get sessions"), http.StatusInternalServerError
	}

	return sessions, nil, http.StatusOK
}

func (s *Service) LogoutSession(ctx context.Context, userID int64, sessionPublicID string) error {

	err := s.tokenRepo.RevokeSession(
		ctx,
		userID,
		sessionPublicID,
		"logout",
		"user",
	)

	if err != nil {
		log.Printf("failed to revoke session: %v", err)
		return fmt.Errorf("failed to revoke session")
	}

	return nil
}

func (s *Service) LogoutAllSessions(ctx context.Context, userID int64, currentSessionID *string) error {

	err := s.tokenRepo.RevokeAllSessions(
		ctx,
		userID,
		currentSessionID,
		"logout_all",
		"user",
	)

	if err != nil {
		log.Printf("failed to revoke all sessions: %v", err)
		return fmt.Errorf("failed to revoke sessions")
	}

	return nil
}
