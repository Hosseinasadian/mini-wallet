package auth

import (
	"context"
	"fmt"
	"github.com/hosseinasadian/mini-wallet/internal/auth/delivery/http"
	authRepository "github.com/hosseinasadian/mini-wallet/internal/auth/repository"
	authService "github.com/hosseinasadian/mini-wallet/internal/auth/service/auth"
	"github.com/hosseinasadian/mini-wallet/pkg/broker"
	"github.com/hosseinasadian/mini-wallet/pkg/config"
	"github.com/hosseinasadian/mini-wallet/pkg/rabbitmq"
	"github.com/jmoiron/sqlx"
	"log"
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
}

func Setup(config Config, db *sqlx.DB) Application {
	authRepo := authRepository.NewRepository(db)

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
		log.Fatal(err)
	}
	//defer rbConn.Close()

	// topology
	topicTopology, err := rabbitmq.NewTopology(rbConn)
	if err != nil {
		log.Fatal(err)
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
		log.Fatal(err)
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
		log.Fatal(err)
	}

	directTopology, err := rabbitmq.NewTopology(rbConn)
	if err != nil {
		log.Fatal(err)
	}
	//defer directTopology.Close()

	err = directTopology.DeclareDirect(rabbitmq.DirectTopologyConfig{
		EventName: "notification",
		RetryTTL:  config.Publisher.RetryTTL,
	})
	if err != nil {
		log.Fatal(err)
	}

	// publisher
	userPublisher, err := rabbitmq.NewTopicPublisher(rbConn, "user")
	if err != nil {
		log.Fatal(err)
	}
	//defer userPublisher.Close()

	notificationPublisher, err := rabbitmq.NewDirectPublisher(rbConn, "notification")
	if err != nil {
		log.Fatal(err)
	}
	//defer notificationPublisher.Close()

	devicePublisher, err := rabbitmq.NewTopicPublisher(rbConn, "device")
	if err != nil {
		log.Fatal(err)
	}
	//defer devicePublisher.Close()

	authSvc := authService.NewService(authRepo, authRepo, authSvcConfig, userPublisher, notificationPublisher)

	httpHandler := http.NewHandler(authSvc, http.Config{
		JWTSecret: config.AuthService.JWTSecret,
	})
	httpServer := http.NewServer(fmt.Sprintf(":%d", config.HTTPPort), httpHandler, config.AuthService.JWTSecret)

	return Application{
		config:                config,
		httpServer:            httpServer,
		userPublisher:         userPublisher,
		notificationPublisher: notificationPublisher,
		devicePublisher:       devicePublisher,
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

	if err := app.userPublisher.Close(); err != nil {
		log.Println(err)
	} else {
		log.Println("user publisher closed")
	}

	if err := app.notificationPublisher.Close(); err != nil {
		log.Println(err)
	} else {
		log.Println("notification publisher closed")
	}

	if err := app.devicePublisher.Close(); err != nil {
		log.Println(err)
	} else {
		log.Println("device publisher closed")
	}

	wg.Wait()
	log.Println("auth app stopped")
}
