package docs

import (
	"context"
	"fmt"
	"github.com/hosseinasadian/mini-wallet/internal/docs/delivery/http"
	"log"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"
)

type Config struct {
	HTTPPort               int               `koanf:"http_port"`
	HTTPShutDownCtxTimeout time.Duration     `koanf:"http_shut_down_timeout"`
	Swagger                http.RoutesConfig `koanf:"swagger"`
}

type Application struct {
	config     Config
	httpServer *http.Server
}

func Setup(config Config) Application {
	httpHandler := http.NewHandler()
	httpServer := http.NewServer(fmt.Sprintf(":%d", config.HTTPPort), httpHandler, config.Swagger)

	return Application{
		config:     config,
		httpServer: httpServer,
	}
}

func (app Application) Start() {
	var wg sync.WaitGroup

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)

	wg.Add(1)
	go func() {
		defer wg.Done()
		app.httpServer.Run()
	}()

	<-stop
	log.Println("⚠️ Received shutdown signal, initiating graceful shutdown...")

	httpCtx, httpCancel := context.WithTimeout(context.Background(), app.config.HTTPShutDownCtxTimeout)
	defer httpCancel()

	if err := app.httpServer.Stop(httpCtx); err != nil {
		log.Printf("HTTP server shutdown error: %v", err)
	}

	wg.Wait()
	log.Println("auth app stopped")
}
