package notification

import (
	"context"
	"fmt"
	"github.com/hosseinasadian/mini-wallet/internal/notification/delivery/http"
	"github.com/hosseinasadian/mini-wallet/internal/notification/service/notification"
	notificationHandler "github.com/hosseinasadian/mini-wallet/internal/notification/subscriber"
	"github.com/hosseinasadian/mini-wallet/pkg/broker"
	"github.com/hosseinasadian/mini-wallet/pkg/hub"
	pkgLogger "github.com/hosseinasadian/mini-wallet/pkg/logger"
	"github.com/hosseinasadian/mini-wallet/pkg/one_signal"
	pkgOtel "github.com/hosseinasadian/mini-wallet/pkg/otel"
	"github.com/hosseinasadian/mini-wallet/pkg/rabbitmq"
	"github.com/hosseinasadian/mini-wallet/pkg/redis"
	"github.com/hosseinasadian/mini-wallet/pkg/sender"
	amqp "github.com/rabbitmq/amqp091-go"
	"go.opentelemetry.io/otel/sdk/metric"
	"log"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"
)

type Config struct {
	HTTPPort                     int                 `koanf:"http_port"`
	HTTPShutDownCtxTimeout       time.Duration       `koanf:"http_shut_down_timeout"`
	JWTSecret                    string              `koanf:"jwt_secret"`
	Redis                        redis.Config        `koanf:"redis"`
	NotificationService          notification.Config `koanf:"notification_service"`
	Subscriber                   broker.Config       `koanf:"subscriber"`
	SubscriberShutdownCtxTimeout time.Duration       `koanf:"subscriber_shutdown_timeout"`
	Sender                       sender.Config       `koanf:"sender"`
	Otel                         pkgOtel.Config      `koanf:"otel"`
}

type Application struct {
	config                 Config
	httpServer             *http.Server
	notificationSubscriber broker.Subscriber
	hub                    *hub.Hub
	notificationHandler    *notificationHandler.Handler
	logger                 *pkgLogger.Logger
}

func Setup(config Config, redisAdapter *redis.Redis, logger *pkgLogger.Logger, mp *metric.MeterProvider) Application {
	mainLogger := logger.With("layer", string(pkgLogger.LayerMain))

	// rabbitMq
	rbConn, err := rabbitmq.NewConnection(config.Subscriber.URL)
	if err != nil {
		mainLogger.Fatal("rabbitmq connection failed", "error", err)
	}
	//defer rbConn.Close()

	topology, err := rabbitmq.NewTopology(rbConn)
	if err != nil {
		mainLogger.Fatal("rabbitmq topology failed", "error", err)
	}
	//defer topology.Close()

	err = topology.DeclareDirect(rabbitmq.DirectTopologyConfig{
		EventName: "auth",
		RetryTTL:  config.Subscriber.RetryTTL,
	})
	if err != nil {
		mainLogger.Fatal("rabbitmq auth event failed", "error", err)
	}

	serviceLogger := logger.With("layer", string(pkgLogger.LayerService))
	notificationSvc := notification.NewService(config.NotificationService, redisAdapter, serviceLogger)
	notificationHub := hub.NewHub(redisAdapter)

	repo := one_signal.FakeRepo{}
	senderOneSignal := one_signal.NewOneSignalSender(config.Sender.OneSignal, &repo)

	httpMetrics, err := pkgOtel.AddHttpMetrics(mp, config.Otel.ServiceName)
	if err != nil {
		mainLogger.Fatal("failed to create http metrics", "error", err)
	}

	httpLogger := logger.With("layer", string(pkgLogger.LayerHTTP))
	httpHandler := http.NewHandler(notificationSvc, notificationHub, httpLogger)
	httpServer := http.NewServer(fmt.Sprintf(":%d", config.HTTPPort), httpHandler, config.JWTSecret, config.Otel.ServiceName, httpLogger, httpMetrics)

	notificationSubscriber, err := rabbitmq.NewDirectSubscriber(
		rbConn,
		"auth",
		rabbitmq.SubscriberConfig{
			Workers:        config.Subscriber.Workers,
			MaxRetry:       config.Subscriber.MaxRetry,
			PrefetchCount:  config.Subscriber.PrefetchCount,
			HandlerTimeout: config.Subscriber.HandlerTimeout,

			OnPanic: func(rec any, msg amqp.Delivery) {
				log.Println("subscriber panic:", rec)
			},

			OnDLQFail: func(msgID string, body []byte, err error) {
				log.Println("dlq publish failed:", err)
			},
		},
	)

	if err != nil {
		mainLogger.Fatal("rabbitmq subscriber failed", "error", err)
	}

	nh := notificationHandler.New(notificationHub, senderOneSignal, logger)

	return Application{
		config:                 config,
		httpServer:             httpServer,
		notificationSubscriber: notificationSubscriber,
		hub:                    notificationHub,
		notificationHandler:    nh,
		logger:                 logger,
	}
}

func (app Application) Start() {
	logger := app.logger
	mainLogger := logger.With("layer", string(pkgLogger.LayerMain))
	var wg sync.WaitGroup

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)

	mainLogger.Info("starting application")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	wg.Add(1)
	go func() {
		defer wg.Done()
		app.hub.StartListener(ctx)
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		app.hub.StartHeartbeat(ctx)
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		err := app.notificationSubscriber.Subscribe(app.notificationHandler.Handle)

		if err != nil {
			mainLogger.Error("subscribe failed", "error", err)
			return
		}

		mainLogger.Info("subscribe done")
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		app.httpServer.Run()
	}()

	<-stop
	mainLogger.Info("received shutdown signal, initiating graceful shutdown")

	httpCtx, httpCancel := context.WithTimeout(ctx, app.config.HTTPShutDownCtxTimeout)
	defer httpCancel()

	if err := app.httpServer.Stop(httpCtx); err != nil {
		mainLogger.Warn("http server stop failed", "error", err)
	}

	subCtx, subCancel := context.WithTimeout(ctx, app.config.SubscriberShutdownCtxTimeout)
	defer subCancel()

	if err := app.notificationSubscriber.Close(subCtx); err != nil {
		mainLogger.Warn("auth publisher close error", "error", err)
	} else {
		mainLogger.Info("auth publisher closed")
	}

	app.hub.Close()

	wg.Wait()
	mainLogger.Info("application stopped")
}
