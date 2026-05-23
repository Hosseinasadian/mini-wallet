package http

import (
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"github.com/gin-gonic/gin"
	docs "github.com/hosseinasadian/mini-wallet/internal/docs/services"
	"github.com/hosseinasadian/mini-wallet/pkg/config"
	"log"
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
	engine     *gin.Engine
	addr       string
	handler    Handler
	httpServer *stdhttp.Server
	routes     RoutesConfig
}

func NewServer(addr string, handler Handler, routes RoutesConfig) *Server {

	s := &Server{
		engine:  gin.Default(),
		addr:    addr,
		handler: handler,
		routes:  routes,
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
