package auth

import (
	"context"
	"fmt"
	"github.com/hosseinasadian/mini-wallet/internal/auth/delivery/http"
	authRepository "github.com/hosseinasadian/mini-wallet/internal/auth/repository"
	authService "github.com/hosseinasadian/mini-wallet/internal/auth/service/auth"
	"github.com/hosseinasadian/mini-wallet/pkg/broker"
	"github.com/hosseinasadian/mini-wallet/pkg/config"
	"github.com/hosseinasadian/mini-wallet/pkg/database"
	pkgLogger "github.com/hosseinasadian/mini-wallet/pkg/logger"
	"github.com/hosseinasadian/mini-wallet/pkg/rabbitmq"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"
)

type Config struct {
	AuthService            authService.Config `koanf:"service"`
	MainRepository         config.MySQL       `koanf:"mysql"`
	HTTPPort               int                `koanf:"http_port"`
	Publisher              broker.Config      `koanf:"publisher"`
	HTTPShutDownCtxTimeout time.Duration      `koanf:"http_shut_down_timeout"`
}

type Application struct {
	config                Config
	httpServer            *http.Server
	userPublisher         broker.TopicPublisher
	notificationPublisher broker.DirectPublisher
	devicePublisher       broker.TopicPublisher
	logger                *pkgLogger.Logger
}

func Setup(config Config, conn *database.Database, logger *pkgLogger.Logger) Application {
	mainLogger := logger.With("layer", string(pkgLogger.LayerMain))

	repoLogger := logger.With("layer", string(pkgLogger.LayerRepository))
	authRepo := authRepository.NewRepository(conn.DB, repoLogger)

	authSvcConfig := authService.Config{
		JWTSecret:            config.AuthService.JWTSecret,
		AccessTokenDuration:  config.AuthService.AccessTokenDuration,
		RefreshTokenDuration: config.AuthService.RefreshTokenDuration,
		EmailRegexp:          config.AuthService.EmailRegexp,
	}

	// rabbit
	// connection
	rbConn, err := rabbitmq.NewConnection(config.Publisher.URL)
	if err != nil {
		mainLogger.Fatal("rabbitmq connection failed", "error", err)
	}
	//defer rbConn.Close()

	// topology
	topicTopology, err := rabbitmq.NewTopology(rbConn)
	if err != nil {
		mainLogger.Fatal("rabbitmq topic topology failed", "error", err)
	}
	//defer topicTopology.Close()

	err = topicTopology.DeclareTopic(rabbitmq.TopicTopologyConfig{
		EventName: "user",
		RetryTTL:  config.Publisher.RetryTTL,
		Bindings: []rabbitmq.TopicBinding{
			{
				Queue:      "wallet-service",
				RoutingKey: "user.created",
			},
		},
	})
	if err != nil {
		mainLogger.Fatal("rabbitmq user topic failed", "error", err)
	}

	err = topicTopology.DeclareTopic(rabbitmq.TopicTopologyConfig{
		EventName: "device",
		RetryTTL:  config.Publisher.RetryTTL,
		Bindings: []rabbitmq.TopicBinding{
			{
				Queue:      "notification-service",
				RoutingKey: "device.register",
			},
		},
	})
	if err != nil {
		mainLogger.Fatal("rabbitmq device topic failed", "error", err)
	}

	directTopology, err := rabbitmq.NewTopology(rbConn)
	if err != nil {
		mainLogger.Fatal("rabbitmq direct topology failed", "error", err)
	}
	//defer directTopology.Close()

	err = directTopology.DeclareDirect(rabbitmq.DirectTopologyConfig{
		EventName: "notification",
		RetryTTL:  config.Publisher.RetryTTL,
	})
	if err != nil {
		mainLogger.Fatal("rabbitmq notification event failed", "error", err)
	}

	// publisher
	userPublisher, err := rabbitmq.NewTopicPublisher(rbConn, "user")
	if err != nil {
		mainLogger.Fatal("rabbitmq user publisher failed", "error", err)
	}
	//defer userPublisher.Close()

	notificationPublisher, err := rabbitmq.NewDirectPublisher(rbConn, "notification")
	if err != nil {
		mainLogger.Fatal("rabbitmq notification publisher failed", "error", err)
	}
	//defer notificationPublisher.Close()

	devicePublisher, err := rabbitmq.NewTopicPublisher(rbConn, "device")
	if err != nil {
		mainLogger.Fatal("rabbitmq device publisher failed", "error", err)
	}
	//defer devicePublisher.Close()

	serviceLogger := logger.With("layer", string(pkgLogger.LayerService))
	authSvc := authService.NewService(authRepo, authRepo, authSvcConfig, userPublisher, notificationPublisher, serviceLogger)

	httpLogger := logger.With("layer", string(pkgLogger.LayerHTTP))
	httpHandler := http.NewHandler(authSvc, http.Config{
		JWTSecret: config.AuthService.JWTSecret,
	}, httpLogger)
	httpServer := http.NewServer(fmt.Sprintf(":%d", config.HTTPPort), httpHandler, config.AuthService.JWTSecret, httpLogger)

	return Application{
		config:                config,
		httpServer:            httpServer,
		userPublisher:         userPublisher,
		notificationPublisher: notificationPublisher,
		devicePublisher:       devicePublisher,
		logger:                logger,
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

	if err := app.userPublisher.Close(); err != nil {
		mainLogger.Warn("user publisher close error", "error", err)
	} else {
		mainLogger.Info("user publisher closed")
	}

	if err := app.notificationPublisher.Close(); err != nil {
		mainLogger.Warn("notification publisher close error", "error", err)
	} else {
		mainLogger.Info("notification publisher closed")
	}

	if err := app.devicePublisher.Close(); err != nil {
		mainLogger.Warn("device publisher close error", "error", err)
	} else {
		mainLogger.Info("device publisher closed")
	}

	wg.Wait()
	mainLogger.Info("application stopped")
}
