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
	"log"
	stdhttp "net/http"
)

type Server struct {
	engine     *gin.Engine
	addr       string
	handler    Handler
	httpServer *stdhttp.Server
}

func NewServer(addr string, handler Handler) *Server {
	s := &Server{
		engine:  gin.Default(),
		addr:    addr,
		handler: handler,
	}

	s.httpServer = &stdhttp.Server{
		Addr:    addr,
		Handler: s.engine,
	}

	s.setRoutes()
	return s
}

func (s *Server) setRoutes() {
	router := s.engine
	router.GET("/ping", s.handler.PingHandler)
	router.POST("/transfer", s.handler.TransferHandler)
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
