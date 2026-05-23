package http

import (
	"github.com/gin-gonic/gin"
	"github.com/hosseinasadian/mini-wallet/internal/auth/service/auth"
	"github.com/hosseinasadian/mini-wallet/pkg/middleware"
	"github.com/hosseinasadian/mini-wallet/pkg/user_access_token"
	"net/http"
	"strings"
)

type Handler struct {
	authService *auth.Service
	config      Config
}

type Config struct {
	JWTSecret string
}

func NewHandler(authService *auth.Service, config Config) Handler {
	return Handler{
		authService: authService,
		config:      config,
	}
}

func (h *Handler) LiveHandler(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"status": "alive",
	})
}

func (h *Handler) ReadyHandler(c *gin.Context) {
	err, code := h.authService.IsReady(c.Request.Context())
	if err != nil {
		c.JSON(code, gin.H{
			"ready":    false,
			"response": err.Error(),
		})
		return
	}

	c.JSON(code, gin.H{
		"ready": true,
	})
}

// RegisterHandler @Summary      Register
// @Description  Register a new user
// @Tags         auth
// @Accept       json
// @Produce      json
// @Param        X-App-Version    header    string              true  "App version"         example(1.4.2)
// @Param        X-Device-Name    header    string              true  "Device name"         example(iPhone 15 Pro)
// @Param        X-Installation-Id header   string              true  "Installation UUID"   example(550e8400-e29b-41d4-a716-446655440000)
// @Param        X-Platform       header    string              true  "Platform"            Enums(ios, android, web)
// @Param        request          body      auth.RegisterRequest true "Register credentials"
// @Success      201              {object}  auth.RegisterResponse
// @Failure      400              {object}  map[string]string
// @Failure      409              {object}  map[string]string
// @Router       /register [post]
func (h *Handler) RegisterHandler(c *gin.Context) {
	var req auth.RegisterRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"message": "invalid request",
		})
		return
	}

	deviceCtx := h.getDeviceContext(c)
	response, err, code := h.authService.Register(c.Request.Context(), deviceCtx, req)
	if err != nil {
		c.JSON(code, gin.H{
			"message": err.Error(),
		})
	} else {
		c.JSON(code, response)
	}
}

// @Summary      Login
// @Description  Authenticate user and return tokens
// @Tags         auth
// @Accept       json
// @Produce      json
// @Param        X-App-Version    header    string              true  "App version"         example(1.4.2)
// @Param        X-Device-Name    header    string              true  "Device name"         example(iPhone 15 Pro)
// @Param        X-Installation-Id header   string              true  "Installation UUID"   example(550e8400-e29b-41d4-a716-446655440000)
// @Param        X-Platform       header    string              true  "Platform"            Enums(ios, android, web)
// @Param        request          body      auth.LoginRequest true "Login credentials"
// @Success      200              {object}  auth.LoginResponse
// @Failure      400              {object}  map[string]string
// @Failure      401              {object}  map[string]string
// @Router       /login [post]
func (h *Handler) LoginHandler(c *gin.Context) {
	var req auth.LoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"message": "invalid request",
		})
		return
	}

	deviceCtx := h.getDeviceContext(c)
	response, err, code := h.authService.Login(c.Request.Context(), deviceCtx, req)
	if err != nil {
		c.JSON(code, gin.H{
			"message": err.Error(),
		})
	} else {
		c.JSON(code, response)
	}
}

// @Summary      Refresh Token
// @Description  Refresh access token using refresh token
// @Tags         auth
// @Produce      json
// @Param        X-App-Version    header    string              true  "App version"         example(1.4.2)
// @Param        X-Device-Name    header    string              true  "Device name"         example(iPhone 15 Pro)
// @Param        X-Installation-Id header   string              true  "Installation UUID"   example(550e8400-e29b-41d4-a716-446655440000)
// @Param        X-Platform       header    string              true  "Platform"            Enums(ios, android, web)
// @Param        Authorization    header    string  true  "Bearer refresh_token"
// @Success      200              {object}  auth.LoginResponse
// @Failure      401              {object}  map[string]string
// @Router       /refresh-token [post]
func (h *Handler) RefreshTokenHandler(c *gin.Context) {
	authHeader := c.GetHeader("Authorization")
	if authHeader == "" {
		c.JSON(http.StatusUnauthorized, gin.H{
			"message": "unauthorized",
		})
		return
	}

	parts := strings.Split(authHeader, " ")
	if len(parts) != 2 || parts[0] != "Bearer" {
		c.JSON(http.StatusUnauthorized, gin.H{
			"message": "unauthorized",
		})
		return
	}

	deviceCtx := h.getDeviceContext(c)
	response, err, code := h.authService.RefreshToken(c.Request.Context(), deviceCtx, parts[1])
	if err != nil {
		c.JSON(code, gin.H{
			"message": err.Error(),
		})
	} else {
		c.JSON(code, response)
	}
}

// @Summary      Verify Token
// @Description  Verify validity of access token
// @Tags         auth
// @Security     BearerAuth
// @Param        Authorization  header    string  true  "Bearer access_token"
// @Success      200
// @Failure      401
// @Router       /verify-token [get]
func (h *Handler) VerifyTokenHandler(c *gin.Context) {
	authHeader := c.GetHeader("Authorization")
	if authHeader == "" {
		c.AbortWithStatus(http.StatusUnauthorized)
		return
	}

	parts := strings.Split(authHeader, " ")
	if len(parts) != 2 || parts[0] != "Bearer" {
		c.AbortWithStatus(http.StatusUnauthorized)
		return
	}

	_, err := user_access_token.VerifyAccessToken(parts[1], h.config.JWTSecret)
	if err != nil {
		c.AbortWithStatus(http.StatusUnauthorized)
		return
	}

	c.Status(http.StatusOK)
}

func (h *Handler) getDeviceContext(c *gin.Context) *auth.DeviceContext {
	userAgent := middleware.GetUserAgent(c)
	ipAddress := middleware.GetIPAddress(c)
	installationId := middleware.GetIdentity(c)
	platform := middleware.GetPlatform(c)
	deviceName := middleware.GetDeviceName(c)
	appVersion := middleware.GetAppVersion(c)

	return &auth.DeviceContext{
		UserAgent:      userAgent,
		IPAddress:      ipAddress,
		InstallationID: installationId,
		Platform:       platform,
		DeviceName:     deviceName,
		AppVersion:     appVersion,
	}
}

// @Summary      Get Sessions
// @Description  Get all active sessions for current user
// @Tags         sessions
// @Security     BearerAuth
// @Produce      json
// @Param        X-App-Version    header    string              true  "App version"         example(1.4.2)
// @Param        X-Device-Name    header    string              true  "Device name"         example(iPhone 15 Pro)
// @Param        X-Installation-Id header   string              true  "Installation UUID"   example(550e8400-e29b-41d4-a716-446655440000)
// @Param        X-Platform       header    string              true  "Platform"            Enums(ios, android, web)
// @Success      200  {object}  map[string]interface{}
// @Failure      401  {object}  map[string]string
// @Failure      500  {object}  map[string]string
// @Router       /sessions [get]
func (h *Handler) GetSessionsHandler(c *gin.Context) {

	userID := middleware.GetUserId(c)

	sessions, err, code := h.authService.GetUserSessions(c.Request.Context(), userID)
	if err != nil {
		c.JSON(code, gin.H{"message": err.Error()})
		return
	}

	c.JSON(code, gin.H{
		"sessions": sessions,
	})
}

// @Summary      Logout Session
// @Description  Logout current session
// @Tags         sessions
// @Security     BearerAuth
// @Param        X-App-Version    header    string              true  "App version"         example(1.4.2)
// @Param        X-Device-Name    header    string              true  "Device name"         example(iPhone 15 Pro)
// @Param        X-Installation-Id header   string              true  "Installation UUID"   example(550e8400-e29b-41d4-a716-446655440000)
// @Param        X-Platform       header    string              true  "Platform"            Enums(ios, android, web)
// @Success      200
// @Failure      401  {object}  map[string]string
// @Failure      500  {object}  map[string]string
// @Router       /logout [post]
func (h *Handler) LogoutSessionHandler(c *gin.Context) {

	userID := middleware.GetUserId(c)
	sessionID := middleware.GetSessionId(c)

	err := h.authService.LogoutSession(
		c.Request.Context(),
		userID,
		sessionID,
	)

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"message": err.Error()})
		return
	}

	c.Status(http.StatusOK)
}

// @Summary      Logout All Sessions
// @Description  Logout all active sessions for current user
// @Tags         sessions
// @Security     BearerAuth
// @Param        X-App-Version    header    string              true  "App version"         example(1.4.2)
// @Param        X-Device-Name    header    string              true  "Device name"         example(iPhone 15 Pro)
// @Param        X-Installation-Id header   string              true  "Installation UUID"   example(550e8400-e29b-41d4-a716-446655440000)
// @Param        X-Platform       header    string              true  "Platform"            Enums(ios, android, web)
// @Success      200
// @Failure      401  {object}  map[string]string
// @Failure      500  {object}  map[string]string
// @Router       /logout-all [post]
func (h *Handler) LogoutAllSessionsHandler(c *gin.Context) {

	userID := middleware.GetUserId(c)
	sessionID := middleware.GetSessionId(c)

	err := h.authService.LogoutAllSessions(
		c.Request.Context(),
		userID,
		&sessionID,
	)

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"message": err.Error()})
		return
	}

	c.Status(http.StatusOK)
}

// @Summary      Revoke Session
// @Description  Revoke a specific session by ID
// @Tags         sessions
// @Security     BearerAuth
// @Produce      json
// @Param        X-App-Version    header    string              true  "App version"         example(1.4.2)
// @Param        X-Device-Name    header    string              true  "Device name"         example(iPhone 15 Pro)
// @Param        X-Installation-Id header   string              true  "Installation UUID"   example(550e8400-e29b-41d4-a716-446655440000)
// @Param        X-Platform       header    string              true  "Platform"            Enums(ios, android, web)
// @Param        session-id  path      string  true  "Session ID"
// @Success      200         {object}  map[string]string
// @Failure      400         {object}  map[string]string
// @Failure      401         {object}  map[string]string
// @Failure      500         {object}  map[string]string
// @Router       /{session-id} [delete]
func (h *Handler) RevokeSessionHandler(c *gin.Context) {

	sessionID := c.Param("session-id")
	if sessionID == "" {
		c.JSON(400, gin.H{
			"message": "session id required",
		})
		return
	}

	userID := middleware.GetUserId(c)

	err := h.authService.LogoutSession(
		c.Request.Context(),
		userID,
		sessionID,
	)

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"message": err.Error()})
		return
	}

	c.JSON(200, gin.H{
		"message": "session revoked successfully",
	})
}
