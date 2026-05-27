package notification

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/hosseinasadian/mini-wallet/internal/notification/delivery/http"
	"github.com/hosseinasadian/mini-wallet/internal/notification/service/notification"
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

type Sender interface {
	Send(ctx context.Context, userID string, title, message string) error
}

type Application struct {
	config                 Config
	httpServer             *http.Server
	notificationSubscriber broker.Subscriber
	hub                    *hub.Hub
	sender                 Sender
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
		EventName: "notification",
		RetryTTL:  config.Subscriber.RetryTTL,
	})
	if err != nil {
		mainLogger.Fatal("rabbitmq notification event failed", "error", err)
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
		"notification",
		rabbitmq.SubscriberConfig{
			Workers:        config.Subscriber.Workers,
			MaxRetry:       config.Subscriber.MaxRetry,
			PrefetchCount:  config.Subscriber.PrefetchCount,
			HandlerTimeout: config.Subscriber.HandlerTimeout,

			OnPanic: func(rec any, msg amqp.Delivery) {
				log.Println("handler panic:", rec)
			},

			OnDLQFail: func(msgID string, body []byte, err error) {
				log.Println("dlq publish failed:", err)
			},
		},
	)

	if err != nil {
		mainLogger.Fatal("rabbitmq subscriber failed", "error", err)
	}

	return Application{config: config, httpServer: httpServer, notificationSubscriber: notificationSubscriber, hub: notificationHub, sender: senderOneSignal, logger: logger}
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
		err := app.notificationSubscriber.Subscribe(func(
			ctx context.Context,
			msg broker.Message,
		) error {

			var evt struct {
				Message string `json:"message"`
				UserID  int64  `json:"user_id"`
			}

			if err := json.Unmarshal(msg.Body, &evt); err != nil {
				return err
			}

			mainLogger.Info("received message", "body", evt.Message, "user_id", evt.UserID)

			userIdStr := fmt.Sprintf("%d", evt.UserID)
			if app.hub.IsOnline(userIdStr) {
				app.hub.Publish(userIdStr, evt.Message)
			} else {
				if err := app.sender.Send(ctx, userIdStr, "new notification", evt.Message); err != nil {
					mainLogger.Error("send notification failed", "error", err)
				}
			}

			return nil
		})

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
		mainLogger.Warn("notification publisher close error", "error", err)
	} else {
		mainLogger.Info("notification publisher closed")
	}

	app.hub.Close()

	wg.Wait()
	mainLogger.Info("application stopped")
}
