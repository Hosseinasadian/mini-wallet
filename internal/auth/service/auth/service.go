package auth

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/hosseinasadian/mini-wallet/pkg/broker"
	"github.com/hosseinasadian/mini-wallet/pkg/httpresponse"
	"github.com/hosseinasadian/mini-wallet/pkg/logger"
	"github.com/hosseinasadian/mini-wallet/pkg/middleware"
	"github.com/hosseinasadian/mini-wallet/pkg/richerror"
	"github.com/hosseinasadian/mini-wallet/pkg/user_access_token"
	"golang.org/x/crypto/bcrypt"
	"log"
	"net/http"
	"regexp"
	"strconv"
	"strings"
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
	emailRegex            *regexp.Regexp
	logger                *logger.Logger
}

func NewService(userRepo UserRepository, tokenRepo TokenRepository, config Config, userPublisher broker.TopicPublisher, notificationPublisher broker.DirectPublisher, logger *logger.Logger) *Service {
	return &Service{
		userRepo:              userRepo,
		tokenRepo:             tokenRepo,
		config:                config,
		userPublisher:         userPublisher,
		notificationPublisher: notificationPublisher,
		emailRegex:            regexp.MustCompile(config.EmailRegexp),
		logger:                logger,
	}
}

func (s *Service) IsReady(ctx context.Context) error {
	const op richerror.Operation = "auth.IsReady"

	urErr := s.userRepo.Ping(ctx)
	if urErr != nil {
		return richerror.New(op).
			WithWrapper(urErr).
			WithMessage("db down").
			WithKind(richerror.KindUnavailable)
	}
	trErr := s.tokenRepo.Ping(ctx)
	if trErr != nil {
		return richerror.New(op).
			WithWrapper(urErr).
			WithMessage("db down").
			WithKind(richerror.KindUnavailable)
	}

	return nil
}

func (s *Service) Register(ctx context.Context, deviceCtx *DeviceContext, req RegisterRequest) (*httpresponse.Response, error) {
	const op richerror.Operation = "auth.Register"
	ctxLogger := middleware.GetLoggerContext(ctx, s.logger)

	ctxLogger.Debug("starting registration", "email", maskEmail(req.Email))

	// validation
	vErr := richerror.New(op).
		WithMessage("validation failed").
		WithKind(richerror.KindUnprocessable)

	if len(req.Password) < 8 {
		vErr = vErr.WithValidation("password", ErrPasswordTooShort)
	}

	if len(req.Password) > 72 {
		vErr = vErr.WithValidation("password", ErrPasswordTooLong)
	}

	if !s.emailRegex.MatchString(req.Email) {
		vErr = vErr.WithValidation("email", ErrInvalidEmail)
	}

	if vErr.HasValidations() {
		ctxLogger.Warn("validation failed", "validations", vErr.Validations())
		return nil, vErr
	}

	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		ctxLogger.Error("bcrypt hashing failed", "error", err)
		return nil, richerror.New(op).
			WithWrapper(err).
			WithMessage(ErrRegisterFailed).
			WithKind(richerror.KindInternal)
	}

	var accountId int64
	accountId, err = s.userRepo.CreateUserByEmailAndPassword(ctx, req.Email, string(hashedPassword))
	if err != nil {
		var re *richerror.RichError
		if errors.As(err, &re) && re.Kind() == richerror.KindConflict {
			ctxLogger.Warn("duplicate email registration", "email", maskEmail(req.Email))

			return nil, richerror.New(op).
				WithWrapper(re).
				WithMessage(ErrEmailAlreadyExists).
				WithKind(richerror.KindConflict)
		}

		ctxLogger.Error("user creation failed", "error", err)

		return nil, richerror.New(op).
			WithWrapper(err).
			WithMessage(ErrRegisterFailed).
			WithKind(richerror.KindInternal)
	}

	ctxLogger.Info("user created successfully", "account_id", accountId, "email", maskEmail(req.Email))

	refreshToken, err := generateRefreshToken()
	if err != nil {
		ctxLogger.Error("refresh token generation failed", "error", err)
		return httpresponse.New(http.StatusCreated, &RegisterResponse{
			Message: "account created but login failed, please login manually",
		}), nil
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
		ctxLogger.Error("session upsert failed", "error", err)
		return httpresponse.New(http.StatusCreated, httpresponse.Response{
			Code: http.StatusCreated,
			Data: &RegisterResponse{
				Message: "account created but login failed, please login manually",
			},
		}), nil
	}

	var accessToken string
	accessToken, err = user_access_token.GenerateAccessToken(accountId, sessionID, s.config.AccessTokenDuration)
	if err != nil {
		ctxLogger.Error("access token generation failed", "error", err)
		return httpresponse.New(http.StatusCreated, &RegisterResponse{
			Message: "account created but login failed, please login manually",
		}), nil
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
			ctxLogger.Warn("event publish failed", "error", err, "event", "user.created")
		} else {
			ctxLogger.Info("event published successfully", "event", "user.created", "account_id", accountId)
		}
	} else {
		ctxLogger.Warn("event marshaling failed", "error", mErr)
	}

	return httpresponse.New(http.StatusCreated, &RegisterResponse{
		Message:      "account successfully created",
		AccessToken:  &accessToken,
		RefreshToken: &refreshToken,
		DeviceID:     deviceID,
		SessionID:    sessionID,
	}), nil
}

func (s *Service) Login(ctx context.Context, deviceCtx *DeviceContext, req LoginRequest) (*httpresponse.Response, error) {
	const op richerror.Operation = "auth.Login"

	// validation
	vErr := richerror.New(op).
		WithMessage("validation failed").
		WithKind(richerror.KindUnprocessable)

	if req.Email == "" {
		vErr = vErr.WithValidation("email", ErrInvalidEmail)
	}

	if req.Password == "" {
		vErr = vErr.WithValidation("password", ErrPasswordRequired)
	}

	if vErr.HasValidations() {
		return nil, vErr
	}

	user, err := s.userRepo.GetUserByEmail(ctx, req.Email)
	if err != nil {
		var re *richerror.RichError
		if errors.As(err, &re) && re.Kind() == richerror.KindNotFound {
			return nil, richerror.New(op).
				WithWrapper(re).
				WithMessage(ErrInvalidLoginCredentials).
				WithKind(richerror.KindUnauthorized)
		}

		return nil, richerror.New(op).
			WithWrapper(err).
			WithMessage(ErrLoginFailed).
			WithKind(richerror.KindInternal)
	}

	err = bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(req.Password))
	if err != nil {
		return nil, richerror.New(op).
			WithWrapper(err).
			WithMessage(ErrInvalidLoginCredentials).
			WithKind(richerror.KindUnauthorized)
	}

	refreshToken, err := generateRefreshToken()
	if err != nil {
		return nil, richerror.New(op).
			WithWrapper(err).
			WithMessage(ErrRefreshTokenFailed).
			WithKind(richerror.KindInternal)
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
		return nil, richerror.New(op).
			WithWrapper(err).
			WithMessage(ErrAccessTokenFailed).
			WithKind(richerror.KindInternal)
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

	return httpresponse.New(http.StatusOK, &LoginResponse{
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
		DeviceID:     deviceID,
		SessionID:    sessionID,
		User:         user,
	}), nil
}

func (s *Service) RefreshToken(ctx context.Context, deviceCtx *DeviceContext, token string) (*httpresponse.Response, error) {
	const op richerror.Operation = "auth.RefreshToken"

	newRefreshToken, err := generateRefreshToken()
	if err != nil {
		return nil, richerror.New(op).
			WithWrapper(err).
			WithMessage(ErrRefreshTokenFailed).
			WithKind(richerror.KindInternal)
	}

	expireRefreshToken := time.Now().Add(s.config.RefreshTokenDuration)
	oldToken := hashRefreshToken(token)
	newToken := hashRefreshToken(newRefreshToken)

	deviceID, sessionID, userID, err := s.tokenRepo.RotateRefreshToken(ctx, deviceCtx, oldToken, newToken, expireRefreshToken)

	accessToken, err := user_access_token.GenerateAccessToken(userID, sessionID, s.config.AccessTokenDuration)
	if err != nil {
		return nil, richerror.New(op).
			WithWrapper(err).
			WithMessage(ErrAccessTokenFailed).
			WithKind(richerror.KindInternal)
	}

	return httpresponse.New(http.StatusOK, &RefreshTokenResponse{
		AccessToken:  accessToken,
		RefreshToken: newRefreshToken,
		DeviceID:     deviceID,
		SessionID:    sessionID,
	}), nil
}

func (s *Service) GetUserSessions(ctx context.Context, userID int64) (*httpresponse.Response, error) {
	const op richerror.Operation = "auth.GetUserSessions"

	sessions, err := s.tokenRepo.GetUserSessions(ctx, userID)
	if err != nil {
		return nil, richerror.New(op).
			WithWrapper(err).
			WithMessage(ErrGetSessionsFailed).
			WithKind(richerror.KindInternal)
	}

	return httpresponse.New(http.StatusOK, sessions), nil
}

func (s *Service) LogoutSession(ctx context.Context, userID int64, sessionPublicID string) error {
	const op richerror.Operation = "auth.LogoutSession"

	err := s.tokenRepo.RevokeSession(
		ctx,
		userID,
		sessionPublicID,
		"logout",
		"user",
	)

	if err != nil {
		return richerror.New(op).
			WithWrapper(err).
			WithMessage(ErrRevokeSessionFailed).
			WithKind(richerror.KindInternal)
	}

	return nil
}

func (s *Service) LogoutAllSessions(ctx context.Context, userID int64, currentSessionID *string) error {
	const op richerror.Operation = "auth.LogoutAllSessions"

	err := s.tokenRepo.RevokeAllSessions(
		ctx,
		userID,
		currentSessionID,
		"logout_all",
		"user",
	)

	if err != nil {
		return richerror.New(op).
			WithWrapper(err).
			WithMessage(ErrRevokeAllSessionsFailed).
			WithKind(richerror.KindInternal)
	}

	return nil
}

func maskEmail(email string) string {
	parts := strings.Split(email, "@")
	if len(parts) != 2 {
		return "***" // fallback
	}
	local := parts[0]
	domain := parts[1]
	if len(local) <= 2 {
		return "***@" + domain
	}
	maskedLocal := local[:2] + strings.Repeat("*", len(local)-2)
	return maskedLocal + "@" + domain
}
