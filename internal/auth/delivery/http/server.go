// @title           Auth Service API
// @version         1.0
// @description     Authentication and session management

// @contact.name    Hossein Asadian
// @contact.email   Hosseinasadian442@email.com

// @host      localhost
// @BasePath  /

// @securityDefinitions.apikey BearerAuth
// @in header
// @name Authorization
package http

import (
	"context"
	"errors"
	"github.com/gin-gonic/gin"
	pkgLogger "github.com/hosseinasadian/mini-wallet/pkg/logger"
	"github.com/hosseinasadian/mini-wallet/pkg/middleware"
	pkgOtel "github.com/hosseinasadian/mini-wallet/pkg/otel"
	stdhttp "net/http"
	"time"
)

type Server struct {
	engine      *gin.Engine
	addr        string
	handler     Handler
	httpServer  *stdhttp.Server
	jwtSecret   string
	serviceName string
	logger      *pkgLogger.Logger
	metrics     *pkgOtel.HTTPMetrics
}

func NewServer(addr string, handler Handler, jwtSecret string, serviceName string, logger *pkgLogger.Logger, metrics *pkgOtel.HTTPMetrics) *Server {

	s := &Server{
		engine:      gin.New(),
		addr:        addr,
		handler:     handler,
		jwtSecret:   jwtSecret,
		serviceName: serviceName,
		logger:      logger,
		metrics:     metrics,
	}

	s.httpServer = &stdhttp.Server{
		Addr:              addr,
		Handler:           s.engine,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       120 * time.Second,
		ReadHeaderTimeout: 10 * time.Second,
	}

	s.setRoutes()
	return s
}

func (s *Server) setRoutes() {
	r := s.engine
	r.Use(
		middleware.OtelMiddleware(s.serviceName),
		middleware.GinSlogLogger(s.logger, s.metrics),
		middleware.GinSlogRecovery(s.logger),
	)

	r.GET("/live", s.handler.LiveHandler)
	r.GET("/ready", s.handler.ReadyHandler)

	// -------------------
	// Public routes
	// -------------------
	public := r.Group("/")
	public.Use(
		middleware.IdentityMiddleware(),
		middleware.DeviceContextMiddleware(),
		middleware.PlatformMiddleware(),
		middleware.AppVersionMiddleware(),
		middleware.DeviceNameMiddleware(),
	)

	public.POST("/register", s.handler.RegisterHandler)
	public.POST("/login", s.handler.LoginHandler)
	public.POST("/refresh-token", s.handler.RefreshTokenHandler)

	// -------------------
	// Auth routes
	// -------------------
	auth := r.Group("/")
	auth.Use(
		middleware.AuthMiddleware(s.jwtSecret),
	)

	auth.GET("/verify-token", s.handler.VerifyTokenHandler)

	sessionsRouter := auth.Group("/")
	sessionsRouter.Use(
		middleware.IdentityMiddleware(),
		middleware.DeviceContextMiddleware(),
		middleware.PlatformMiddleware(),
		middleware.AppVersionMiddleware(),
		middleware.DeviceNameMiddleware(),
	)

	sessionsRouter.GET("/sessions", s.handler.GetSessionsHandler)
	sessionsRouter.POST("/logout", s.handler.LogoutSessionHandler)
	sessionsRouter.POST("/logout-all", s.handler.LogoutAllSessionsHandler)
	sessionsRouter.DELETE("/:session-id", s.handler.RevokeSessionHandler)

}

func (s *Server) Run() {
	s.logger.Info("http server started", "addr", s.addr)
	if err := s.httpServer.ListenAndServe(); err != nil && !errors.Is(err, stdhttp.ErrServerClosed) {
		s.logger.Debug("http server stopped", "err", err)
	}
}

func (s *Server) Stop(ctx context.Context) error {
	if err := s.httpServer.Shutdown(ctx); err != nil {
		return err
	}

	s.logger.Info("http server stopped", "addr", s.addr)
	return nil
}
