// @title           Wallet Service API
// @version         1.0
// @description     Wallet Management

// @contact.name    Hossein Asadian
// @contact.email   Hosseinasadian442@email.com

// @host      localhost
// @BasePath  /
package http

import (
	"context"
	"errors"
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
	logger     *pkgLogger.Logger
}

func NewServer(addr string, handler Handler, logger *pkgLogger.Logger) *Server {
	s := &Server{
		engine:  gin.New(),
		addr:    addr,
		handler: handler,
		logger:  logger,
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

	router.GET("/live", s.handler.LiveHandler)
	router.GET("/ready", s.handler.ReadyHandler)

	router.POST("/transfer", s.handler.TransferHandler)
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
