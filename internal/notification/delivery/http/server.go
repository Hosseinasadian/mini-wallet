// @title           Notification Service API
// @version         1.0
// @description     Notification management

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
	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	pkgLogger "github.com/hosseinasadian/mini-wallet/pkg/logger"
	"github.com/hosseinasadian/mini-wallet/pkg/middleware"
	stdhttp "net/http"
	"time"
)

type Server struct {
	engine     *gin.Engine
	addr       string
	handler    Handler
	httpServer *stdhttp.Server
	jwtSecret  string
	logger     *pkgLogger.Logger
}

func NewServer(addr string, handler Handler, jwtSecret string, logger *pkgLogger.Logger) *Server {
	s := &Server{
		engine:    gin.New(),
		addr:      addr,
		handler:   handler,
		jwtSecret: jwtSecret,
		logger:    logger,
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
	router := s.engine
	router.Use(middleware.GinSlogLogger(s.logger), middleware.GinSlogRecovery(s.logger))

	router.Use(cors.New(cors.Config{
		AllowAllOrigins: true,
		AllowMethods: []string{
			"GET",
			"POST",
			"OPTIONS",
		},
		AllowHeaders: []string{
			"Origin",
			"Content-Type",
			"Authorization",
		},
	}))

	router.GET("/live", s.handler.LiveHandler)
	router.GET("/ready", s.handler.ReadyHandler)

	router.GET("/stream", s.handler.NotificationsHandler)

	authRouter := router.Use(middleware.AuthMiddleware(s.jwtSecret))

	authRouter.POST("/ticket", s.handler.TicketHandler)
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
