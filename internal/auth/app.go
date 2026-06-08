package auth

import (
	"context"
	"fmt"
	authGrpc "github.com/hosseinasadian/mini-wallet/internal/auth/delivery/grpc"
	"github.com/hosseinasadian/mini-wallet/internal/auth/delivery/http"
	authRepository "github.com/hosseinasadian/mini-wallet/internal/auth/repository"
	authService "github.com/hosseinasadian/mini-wallet/internal/auth/service/auth"
	outoboxService "github.com/hosseinasadian/mini-wallet/internal/auth/service/outbox"
	outoboxWorker "github.com/hosseinasadian/mini-wallet/internal/auth/workers/outbox"
	"github.com/hosseinasadian/mini-wallet/pkg/broker"
	"github.com/hosseinasadian/mini-wallet/pkg/config"
	"github.com/hosseinasadian/mini-wallet/pkg/database"
	"github.com/hosseinasadian/mini-wallet/pkg/grpc"
	pkgGrpc "github.com/hosseinasadian/mini-wallet/pkg/grpc"
	"github.com/hosseinasadian/mini-wallet/pkg/grpc/interceptors"
	pkgLogger "github.com/hosseinasadian/mini-wallet/pkg/logger"
	pkgOtel "github.com/hosseinasadian/mini-wallet/pkg/otel"
	"github.com/hosseinasadian/mini-wallet/pkg/rabbitmq"
	"go.opentelemetry.io/otel/sdk/metric"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"
)

//BatchInterval    time.Duration `koanf:"batch_interval"`
//RecoveryInterval time.Duration `koanf:"recovery_interval"`
//LockTimeout      time.Duration `koanf:"lock_timeout"`
//BatchSize        int64         `koanf:"batch_size"`
//Concurrency      int64         `koanf:"concurrency"`

type Config struct {
	AuthService            authService.Config         `koanf:"service"`
	MainRepository         config.MySQL               `koanf:"mysql"`
	HTTPPort               int                        `koanf:"http_port"`
	Publisher              broker.Config              `koanf:"publisher"`
	HTTPShutDownCtxTimeout time.Duration              `koanf:"http_shut_down_timeout"`
	Otel                   pkgOtel.Config             `koanf:"otel"`
	OutboxWorker           outoboxWorker.WorkerConfig `koanf:"outbox_worker"`
	OutboxService          outoboxService.Config      `koanf:"outbox_service"`
	GRPCServer             pkgGrpc.Config             `koanf:"grpc_server"`
}

type Application struct {
	config                Config
	httpServer            *http.Server
	userPublisher         broker.TopicPublisher
	notificationPublisher broker.DirectPublisher
	devicePublisher       broker.TopicPublisher
	logger                *pkgLogger.Logger
	worker                *outoboxWorker.Worker
	grpcServer            authGrpc.Server
}

func Setup(config Config, conn *database.Database, logger *pkgLogger.Logger, mp *metric.MeterProvider) Application {
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

	directTopology, err := rabbitmq.NewTopology(rbConn)
	if err != nil {
		mainLogger.Fatal("rabbitmq direct topology failed", "error", err)
	}
	//defer directTopology.Close()

	err = directTopology.DeclareDirect(rabbitmq.DirectTopologyConfig{
		EventName: "auth",
		RetryTTL:  config.Publisher.RetryTTL,
	})
	if err != nil {
		mainLogger.Fatal("rabbitmq auth event failed", "error", err)
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
	outboxSvc := outoboxService.NewService(authRepo, config.OutboxService, userPublisher, notificationPublisher, serviceLogger)
	authSvc := authService.NewService(authRepo, authSvcConfig, outboxSvc, serviceLogger)

	httpMetrics, err := pkgOtel.AddHttpMetrics(mp, config.Otel.ServiceName)
	if err != nil {
		mainLogger.Fatal("failed to create http metrics", "error", err)
	}

	httpLogger := logger.With("layer", string(pkgLogger.LayerHTTP))
	httpHandler := http.NewHandler(authSvc, http.Config{
		JWTSecret: config.AuthService.JWTSecret,
	}, httpLogger)
	httpServer := http.NewServer(fmt.Sprintf(":%d", config.HTTPPort), httpHandler, config.AuthService.JWTSecret, config.Otel.ServiceName, httpLogger, httpMetrics)

	outboxW := outoboxWorker.NewWorker(outboxSvc, serviceLogger, config.OutboxWorker)

	grpcMetrics, err := interceptors.NewGRPCMetrics(mp, config.Otel.ServiceName)
	if err != nil {
		mainLogger.Fatal("failed to create grpc metrics", "error", err)
	}

	grpcInterceptors := interceptors.New(config.Otel.ServiceName, serviceLogger, grpcMetrics)
	grpcServer, err := grpc.New(config.GRPCServer, grpcInterceptors)

	authGrpcHandler := authGrpc.NewHandler(authSvc)
	authGrpcServer := authGrpc.New(grpcServer, authGrpcHandler)

	return Application{
		config:                config,
		httpServer:            httpServer,
		userPublisher:         userPublisher,
		notificationPublisher: notificationPublisher,
		devicePublisher:       devicePublisher,
		logger:                logger,
		worker:                outboxW,
		grpcServer:            authGrpcServer,
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

	wg.Add(1)
	go func() {
		defer wg.Done()

		mainLogger.Info("gRPC server starting", "port", app.config.GRPCServer.Port)
		if err := app.grpcServer.Serve(); err != nil {
			mainLogger.Error("error in serving gRPC server", "error", err)
		}
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

	httpCtx, httpCancel := context.WithTimeout(context.Background(), app.config.HTTPShutDownCtxTimeout)
	defer httpCancel()

	if err := app.httpServer.Stop(httpCtx); err != nil {
		mainLogger.Warn("http server stop failed", "error", err)
	}

	logger.Info("Shutting down gRPC server...")
	app.grpcServer.Stop()
	logger.Info("gRPC server stopped")

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

	workerCancel()

	wg.Wait()
	mainLogger.Info("application stopped")
}
