package auth

import (
	"context"
	"errors"
	"fmt"
	"github.com/google/uuid"
	authpb "github.com/hosseinasadian/mini-wallet/gen/go/auth/v1"
	"github.com/hosseinasadian/mini-wallet/internal/auth/service/outbox"
	"github.com/hosseinasadian/mini-wallet/pkg/event"
	"google.golang.org/protobuf/proto"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/hosseinasadian/mini-wallet/pkg/httpresponse"
	"github.com/hosseinasadian/mini-wallet/pkg/logger"
	"github.com/hosseinasadian/mini-wallet/pkg/middleware"
	"github.com/hosseinasadian/mini-wallet/pkg/richerror"
	"github.com/hosseinasadian/mini-wallet/pkg/user_access_token"
	"golang.org/x/crypto/bcrypt"
)

type Config struct {
	JWTSecret            string        `koanf:"jwt_secret"`
	AccessTokenDuration  time.Duration `koanf:"access_token_duration"`
	RefreshTokenDuration time.Duration `koanf:"refresh_token_duration"`
	EmailRegexp          string        `koanf:"email_regexp"`
}

type Service struct {
	repo       RepositoryTx
	config     Config
	emailRegex *regexp.Regexp
	logger     *logger.Logger
	outbox     *outbox.Service
}

func NewService(repo RepositoryTx, config Config, outbox *outbox.Service, logger *logger.Logger) *Service {
	return &Service{
		repo:       repo,
		config:     config,
		emailRegex: regexp.MustCompile(config.EmailRegexp),
		logger:     logger,
		outbox:     outbox,
	}
}

func (s *Service) IsReady(ctx context.Context) error {
	const op richerror.Operation = "auth.IsReady"

	urErr := s.repo.Ping(ctx)
	if urErr != nil {
		return richerror.New(op).
			WithWrapper(urErr).
			WithMessage("db down").
			WithKind(richerror.KindUnavailable)
	}
	trErr := s.repo.Ping(ctx)
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
	var accessToken, refreshToken, deviceID, sessionID string
	var registerEventBody []byte
	var outboxEvent *outbox.OutboxEvent

	tErr := s.repo.RunInTx(ctx, func(exec Repository) error {
		accountId, err = exec.CreateUserByEmailAndPassword(ctx, req.Email, string(hashedPassword))
		if err != nil {
			var re *richerror.RichError
			if errors.As(err, &re) && re.Kind() == richerror.KindConflict {
				ctxLogger.Warn("duplicate email registration", "email", maskEmail(req.Email))

				return richerror.New(op).
					WithWrapper(re).
					WithMessage(ErrEmailAlreadyExists).
					WithKind(richerror.KindConflict)
			}

			ctxLogger.Error("user creation failed", "error", err)

			return richerror.New(op).
				WithWrapper(err).
				WithMessage(ErrRegisterFailed).
				WithKind(richerror.KindInternal)
		}

		refreshToken, err = generateRefreshToken()
		if err != nil {
			ctxLogger.Error("refresh token generation failed", "error", err)
			return richerror.New(op).
				WithWrapper(err).
				WithMessage(ErrRegisterFailed).
				WithKind(richerror.KindInternal)
		}

		newToken := hashRefreshToken(refreshToken)
		expireRefreshToken := time.Now().Add(s.config.RefreshTokenDuration)

		deviceID, sessionID, err = exec.UpsertSession(
			ctx,
			deviceCtx,
			accountId,
			newToken,
			expireRefreshToken,
			req.PushToken,
		)
		if err != nil {
			ctxLogger.Error("session upsert failed", "error", err)
			return richerror.New(op).
				WithWrapper(err).
				WithMessage(ErrRegisterFailed).
				WithKind(richerror.KindInternal)
		}

		accessToken, err = user_access_token.GenerateAccessToken(accountId, sessionID, s.config.JWTSecret, s.config.AccessTokenDuration)
		if err != nil {
			ctxLogger.Error("access token generation failed", "error", err)
			return richerror.New(op).
				WithWrapper(err).
				WithMessage(ErrRegisterFailed).
				WithKind(richerror.KindInternal)
		}

		// publish event
		newRegisterEvent := &authpb.RegisterNewUser{
			Id:    accountId,
			Email: req.Email,
		}
		registerToCommon, err := event.New(newRegisterEvent, event.TypeAuthRegisterNewUser)

		if err != nil {
			s.logger.Error("failed to create register event", "error", err)
			return richerror.New(op).
				WithWrapper(err).
				WithMessage(ErrRegisterFailed).
				WithKind(richerror.KindInternal)
		}

		registerEventBody, err = proto.Marshal(registerToCommon)
		if err != nil {
			ctxLogger.Error("event marshaling failed", "error", err)
			return richerror.New(op).
				WithWrapper(err).
				WithMessage(ErrRegisterFailed).
				WithKind(richerror.KindInternal)
		}

		outboxEvent = &outbox.OutboxEvent{
			EventID:       uuid.New().String(),
			EventType:     string(event.TypeAuthRegisterNewUser),
			Payload:       registerEventBody,
			AggregateType: "user",
			AggregateID:   fmt.Sprintf("%d", accountId),
			CreatedAt:     time.Now(),
		}

		err = exec.InsertOutboxEvent(ctx, outboxEvent)
		if err != nil {
			ctxLogger.Error("event insert failed", "error", err)
			return richerror.New(op).
				WithWrapper(err).
				WithMessage(ErrRegisterFailed).
				WithKind(richerror.KindInternal)
		}

		return nil
	})

	if tErr != nil {
		return nil, richerror.New(op).
			WithWrapper(tErr).
			WithMessage(ErrRegisterFailed).
			WithKind(richerror.KindInternal)
	}

	ctxLogger.Info("user created successfully", "account_id", accountId, "email", maskEmail(req.Email))

	go func() {
		defer func() {
			if r := recover(); r != nil {
				s.logger.Error("panic in outbox immediate publisher", "recover", r)
			}
		}()

		bgCtx := context.Background()

		err = s.outbox.ProcessSingleEvent(bgCtx, outboxEvent)

		if err != nil {
			s.logger.Error("outbox single event process failed", "error", err)
		}
	}()

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
	ctxLogger := middleware.GetLoggerContext(ctx, s.logger)

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

	user, err := s.repo.GetUserByEmail(ctx, req.Email)
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

	var accessToken, refreshToken, deviceID, sessionID string
	var outboxEvent *outbox.OutboxEvent
	var newLoggedEventBody []byte

	tErr := s.repo.RunInTx(ctx, func(exec Repository) error {
		refreshToken, err = generateRefreshToken()
		if err != nil {
			return richerror.New(op).
				WithWrapper(err).
				WithMessage(ErrRefreshTokenFailed).
				WithKind(richerror.KindInternal)
		}

		expireRefreshToken := time.Now().Add(s.config.RefreshTokenDuration)
		newToken := hashRefreshToken(refreshToken)
		deviceID, sessionID, err = exec.UpsertSession(
			ctx,
			deviceCtx,
			user.ID,
			newToken,
			expireRefreshToken,
			req.PushToken,
		)

		accessToken, err = user_access_token.GenerateAccessToken(user.ID, sessionID, s.config.JWTSecret, s.config.AccessTokenDuration)
		if err != nil {
			return richerror.New(op).
				WithWrapper(err).
				WithMessage(ErrAccessTokenFailed).
				WithKind(richerror.KindInternal)
		}

		// publish event
		newLoggedEvent := &authpb.NewSessionLoggedIn{
			Id:     sessionID,
			UserId: user.ID,
		}
		newLoggedToCommon, err := event.New(newLoggedEvent, event.TypeAuthNewSessionLoggedIn)

		if err != nil {
			s.logger.Error("failed to create new logged event", "error", err)
			return richerror.New(op).
				WithWrapper(err).
				WithMessage(ErrLoginFailed).
				WithKind(richerror.KindInternal)
		}

		newLoggedEventBody, err = proto.Marshal(newLoggedToCommon)
		if err != nil {
			ctxLogger.Warn("event marshaling failed", "error", err)
			return richerror.New(op).
				WithWrapper(err).
				WithMessage(ErrLoginFailed).
				WithKind(richerror.KindInternal)
		}

		outboxEvent = &outbox.OutboxEvent{
			EventID:       uuid.New().String(),
			EventType:     string(event.TypeAuthNewSessionLoggedIn),
			Payload:       newLoggedEventBody,
			AggregateType: "user",
			AggregateID:   fmt.Sprintf("%d", user.ID),
			CreatedAt:     time.Now(),
		}

		err = exec.InsertOutboxEvent(ctx, outboxEvent)
		if err != nil {
			ctxLogger.Error("event insert failed", "error", err)
			return richerror.New(op).
				WithWrapper(err).
				WithMessage(ErrLoginFailed).
				WithKind(richerror.KindInternal)
		}

		return nil

	})

	if tErr != nil {
		return nil, richerror.New(op).
			WithWrapper(tErr).
			WithMessage(ErrLoginFailed).
			WithKind(richerror.KindInternal)
	}

	ctxLogger.Info("user created successfully", "account_id", user.ID, "email", maskEmail(req.Email))

	go func() {
		defer func() {
			if r := recover(); r != nil {
				s.logger.Error("panic in outbox immediate publisher", "recover", r)
			}
		}()

		bgCtx := context.Background()

		err = s.outbox.ProcessSingleEvent(bgCtx, outboxEvent)

		if err != nil {
			s.logger.Error("outbox single event process failed", "error", err)
		}

	}()

	return httpresponse.New(http.StatusOK, &LoginResponse{
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
		DeviceID:     deviceID,
		SessionID:    sessionID,
		User:         user,
	}), nil
}

func (s *Service) RefreshToken(ctx context.Context, deviceCtx *DeviceContext, req RefreshTokenRequest) (*httpresponse.Response, error) {
	const op richerror.Operation = "auth.RefreshToken"

	newRefreshToken, err := generateRefreshToken()
	if err != nil {
		return nil, richerror.New(op).
			WithWrapper(err).
			WithMessage(ErrRefreshTokenFailed).
			WithKind(richerror.KindInternal)
	}

	expireRefreshToken := time.Now().Add(s.config.RefreshTokenDuration)
	oldToken := hashRefreshToken(req.RefreshToken)
	newToken := hashRefreshToken(newRefreshToken)

	deviceID, sessionID, userID, err := s.repo.RotateRefreshToken(ctx, deviceCtx, oldToken, newToken, expireRefreshToken, req.PushToken)
	if err != nil {
		var re *richerror.RichError
		if errors.As(err, &re) && re.Kind() == richerror.KindNotFound {
			return nil, richerror.New(op).
				WithWrapper(re).
				WithMessage(ErrRefreshTokenFailed).
				WithKind(richerror.KindUnauthorized)
		}

		return nil, richerror.New(op).
			WithWrapper(err).
			WithMessage(ErrRefreshTokenFailed).
			WithKind(richerror.KindInternal)
	}

	accessToken, err := user_access_token.GenerateAccessToken(userID, sessionID, s.config.JWTSecret, s.config.AccessTokenDuration)
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

	sessions, err := s.repo.GetUserSessions(ctx, userID)
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

	err := s.repo.RevokeSession(
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

	err := s.repo.RevokeAllSessions(
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

func (s *Service) UpdatePushToken(ctx context.Context, userID int64, sessionID, pushToken string) error {
	const op = "auth.UpdatePushToken"

	err := s.repo.UpdatePushToken(ctx, sessionID, userID, pushToken)
	if err != nil {
		var re *richerror.RichError
		if errors.As(err, &re) && re.Kind() == richerror.KindNotFound {
			return richerror.New(op).
				WithWrapper(err).
				WithMessage("session not found").
				WithKind(richerror.KindNotFound)
		}
		return richerror.New(op).
			WithWrapper(err).
			WithMessage("failed to update push token").
			WithKind(richerror.KindInternal)
	}

	return nil
}

func (s *Service) GetActiveSessions(ctx context.Context, userID int64) ([]SessionItem, error) {
	const op richerror.Operation = "auth.GetActiveSessions"

	sessions, err := s.repo.GetUserSessions(ctx, userID)
	if err != nil {
		return nil, richerror.New(op).
			WithWrapper(err).
			WithMessage(ErrGetSessionsFailed).
			WithKind(richerror.KindInternal)
	}

	return sessions, nil
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
