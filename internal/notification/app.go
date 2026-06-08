package notification

import (
	"context"
	"fmt"
	authAdapter "github.com/hosseinasadian/mini-wallet/adapter/notification/auth"
	"github.com/hosseinasadian/mini-wallet/internal/notification/delivery/http"
	notifRepository "github.com/hosseinasadian/mini-wallet/internal/notification/repository"
	eventService "github.com/hosseinasadian/mini-wallet/internal/notification/service/event"
	"github.com/hosseinasadian/mini-wallet/internal/notification/service/notification"
	"github.com/hosseinasadian/mini-wallet/internal/notification/service/outbox"
	notificationHandler "github.com/hosseinasadian/mini-wallet/internal/notification/subscriber"
	outoboxWorker "github.com/hosseinasadian/mini-wallet/internal/notification/workers/outbox"
	"github.com/hosseinasadian/mini-wallet/pkg/broker"
	"github.com/hosseinasadian/mini-wallet/pkg/config"
	"github.com/hosseinasadian/mini-wallet/pkg/database"
	pkgGrpc "github.com/hosseinasadian/mini-wallet/pkg/grpc"
	"github.com/hosseinasadian/mini-wallet/pkg/grpc/interceptors"
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
	HTTPPort                     int                        `koanf:"http_port"`
	MainRepository               config.MySQL               `koanf:"mysql"`
	HTTPShutDownCtxTimeout       time.Duration              `koanf:"http_shut_down_timeout"`
	JWTSecret                    string                     `koanf:"jwt_secret"`
	Redis                        redis.Config               `koanf:"redis"`
	NotificationService          notification.Config        `koanf:"notification_service"`
	Subscriber                   broker.Config              `koanf:"subscriber"`
	SubscriberShutdownCtxTimeout time.Duration              `koanf:"subscriber_shutdown_timeout"`
	Sender                       sender.Config              `koanf:"sender"`
	Otel                         pkgOtel.Config             `koanf:"otel"`
	GrpcNotificationClient       pkgGrpc.Client             `koanf:"grpc_notification_client"`
	OutboxService                outbox.Config              `koanf:"outbox_service"`
	OutboxWorker                 outoboxWorker.WorkerConfig `koanf:"outbox_worker"`
}

type Application struct {
	config                 Config
	httpServer             *http.Server
	notificationSubscriber broker.Subscriber
	notificationPublisher  broker.DirectPublisher
	hub                    *hub.Hub
	notificationHandler    *notificationHandler.Handler
	logger                 *pkgLogger.Logger
	worker                 *outoboxWorker.Worker
}

func Setup(config Config, conn *database.Database, redisAdapter *redis.Redis, logger *pkgLogger.Logger, mp *metric.MeterProvider) Application {
	mainLogger := logger.With("layer", string(pkgLogger.LayerMain))

	repoLogger := logger.With("layer", string(pkgLogger.LayerRepository))
	notifRepo := notifRepository.NewRepository(conn.DB, repoLogger)

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
				log.Println("subscriber panic:", rec)
			},

			OnDLQFail: func(msgID string, body []byte, err error) {
				log.Println("dlq publish failed:", err)
			},
		},
	)

	notificationPublisher, err := rabbitmq.NewDirectPublisher(rbConn, "notification")
	if err != nil {
		mainLogger.Fatal("rabbitmq notification publisher failed", "error", err)
	}

	if err != nil {
		mainLogger.Fatal("rabbitmq subscriber failed", "error", err)
	}

	grpcMetrics, err := interceptors.NewGRPCMetrics(mp, config.Otel.ServiceName)
	if err != nil {
		mainLogger.Fatal("failed to create grpc metrics", "error", err)
	}

	grpcInterceptors := interceptors.New(config.Otel.ServiceName, serviceLogger, grpcMetrics)
	authGrpcClient, err := pkgGrpc.NewClient(config.GrpcNotificationClient, grpcInterceptors)

	if err != nil {
		mainLogger.Fatal("failed to connect auth grpc server", "error", err)
	}

	authClient := authAdapter.New(authGrpcClient)

	outboxSvc := outbox.NewService(notifRepo, config.OutboxService, notificationPublisher, serviceLogger)

	eventSvc := eventService.NewService(notifRepo, serviceLogger)

	nh := notificationHandler.New(notificationHub, authClient, senderOneSignal, outboxSvc, eventSvc, serviceLogger)

	outboxW := outoboxWorker.NewWorker(outboxSvc, serviceLogger, config.OutboxWorker)

	return Application{
		config:                 config,
		httpServer:             httpServer,
		notificationSubscriber: notificationSubscriber,
		notificationPublisher:  notificationPublisher,
		hub:                    notificationHub,
		notificationHandler:    nh,
		logger:                 logger,
		worker:                 outboxW,
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

	workerCtx, workerCancel := context.WithCancel(context.Background())
	defer workerCancel()

	wg.Add(1)
	go func() {
		defer wg.Done()
		app.worker.Start(workerCtx)
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

	if err := app.notificationPublisher.Close(); err != nil {
		mainLogger.Warn("notification publisher close error", "error", err)
	} else {
		mainLogger.Info("notification publisher closed")
	}

	if err := app.notificationSubscriber.Close(subCtx); err != nil {
		mainLogger.Warn("notification publisher close error", "error", err)
	} else {
		mainLogger.Info("notification publisher closed")
	}

	app.hub.Close()

	workerCancel()

	wg.Wait()
	mainLogger.Info("application stopped")
}
