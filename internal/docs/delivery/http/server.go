package http

import (
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"github.com/gin-gonic/gin"
	docs "github.com/hosseinasadian/mini-wallet/internal/docs/services"
	"github.com/hosseinasadian/mini-wallet/pkg/config"
	"github.com/hosseinasadian/mini-wallet/pkg/logger"
	"github.com/hosseinasadian/mini-wallet/pkg/middleware"
	pkgOtel "github.com/hosseinasadian/mini-wallet/pkg/otel"
	stdhttp "net/http"
	"strings"
	"time"
)

type RoutesConfig struct {
	Auth         config.Swagger `koanf:"auth"`
	Wallet       config.Swagger `koanf:"wallet"`
	Notification config.Swagger `koanf:"notification"`
}

type Server struct {
	engine      *gin.Engine
	addr        string
	handler     Handler
	httpServer  *stdhttp.Server
	serviceName string
	routes      RoutesConfig
	logger      *logger.Logger
	metrics     *pkgOtel.HTTPMetrics
}

func NewServer(addr string, handler Handler, routes RoutesConfig, serviceName string, logger *logger.Logger, metrics *pkgOtel.HTTPMetrics) *Server {

	s := &Server{
		engine:      gin.New(),
		addr:        addr,
		handler:     handler,
		routes:      routes,
		logger:      logger,
		serviceName: serviceName,
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

	r.GET("/api-list", func(c *gin.Context) {
		c.Data(
			stdhttp.StatusOK,
			"text/html; charset=utf-8",
			docs.Html,
		)
	})

	r.GET("/auth", s.patchAndServe(
		docs.AuthSpec,
		s.routes.Auth.Host,
		strings.Split(s.routes.Auth.Schemes, ","),
	))

	r.GET("/wallet", s.patchAndServe(
		docs.WalletSpec,
		s.routes.Wallet.Host,
		strings.Split(s.routes.Wallet.Schemes, ","),
	))

	r.GET("/notification", s.patchAndServe(
		docs.NotificationSpec,
		s.routes.Notification.Host,
		strings.Split(s.routes.Notification.Schemes, ","),
	))

	r.GET("/config", func(c *gin.Context) {
		c.JSON(stdhttp.StatusOK, gin.H{
			"services": []gin.H{
				{"name": "Auth", "url": "/docs/auth"},
				{"name": "Wallet", "url": "/docs/wallet"},
				{"name": "Notification", "url": "/docs/notification"},
			},
		})
	})
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

func (s *Server) patchAndServe(spec []byte, host string, schemes []string) gin.HandlerFunc {
	return func(c *gin.Context) {
		var doc map[string]any
		err := json.Unmarshal(spec, &doc)
		if err != nil {
			return
		}

		doc["host"] = host
		doc["schemes"] = schemes

		c.JSON(stdhttp.StatusOK, doc)
	}
}
