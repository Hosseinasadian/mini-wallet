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
	"github.com/hosseinasadian/mini-wallet/pkg/middleware"
	"log"
	stdhttp "net/http"
)

type Server struct {
	engine     *gin.Engine
	addr       string
	handler    Handler
	httpServer *stdhttp.Server
	jwtSecret  string
}

func NewServer(addr string, handler Handler, jwtSecret string) *Server {

	s := &Server{
		engine:    gin.Default(),
		addr:      addr,
		handler:   handler,
		jwtSecret: jwtSecret,
	}

	s.httpServer = &stdhttp.Server{
		Addr:    addr,
		Handler: s.engine,
	}

	s.setRoutes()
	return s
}

func (s *Server) setRoutes() {
	r := s.engine

	r.GET("/ping", s.handler.PingHandler)

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
	log.Printf("HTTP server starting on %s", s.addr)
	if err := s.httpServer.ListenAndServe(); err != nil && !errors.Is(err, stdhttp.ErrServerClosed) {
		log.Printf("HTTP server error: %v", err)
	}
}

func (s *Server) Stop(ctx context.Context) error {
	if err := s.httpServer.Shutdown(ctx); err != nil {
		return err
	}

	log.Printf("HTTP server stopped on %s", s.addr)
	return nil
}
