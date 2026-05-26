package docs

import (
	"context"
	"fmt"
	"github.com/hosseinasadian/mini-wallet/internal/docs/delivery/http"
	pkgLogger "github.com/hosseinasadian/mini-wallet/pkg/logger"
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
	logger     *pkgLogger.Logger
}

func Setup(config Config, logger *pkgLogger.Logger) Application {
	httpLogger := logger.With("layer", string(pkgLogger.LayerHTTP))
	httpHandler := http.NewHandler(httpLogger)
	httpServer := http.NewServer(fmt.Sprintf(":%d", config.HTTPPort), httpHandler, config.Swagger, httpLogger)

	return Application{
		config:     config,
		httpServer: httpServer,
		logger:     logger,
	}
}

func (app Application) Start() {
	logger := app.logger
	mainLogger := logger.With("layer", string(pkgLogger.LayerMain))
	var wg sync.WaitGroup

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)

	mainLogger.Info("starting application")

	wg.Add(1)
	go func() {
		defer wg.Done()
		app.httpServer.Run()
	}()

	<-stop
	mainLogger.Info("received shutdown signal, initiating graceful shutdown")

	httpCtx, httpCancel := context.WithTimeout(context.Background(), app.config.HTTPShutDownCtxTimeout)
	defer httpCancel()

	if err := app.httpServer.Stop(httpCtx); err != nil {
		mainLogger.Warn("http server stop failed", "error", err)
	}

	wg.Wait()
	mainLogger.Info("application stopped")
}
