package wallet

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/hosseinasadian/mini-wallet/internal/wallet/delivery/http"
	walletRepository "github.com/hosseinasadian/mini-wallet/internal/wallet/repository"
	walletService "github.com/hosseinasadian/mini-wallet/internal/wallet/service"
	"github.com/hosseinasadian/mini-wallet/pkg/broker"
	"github.com/hosseinasadian/mini-wallet/pkg/config"
	"github.com/hosseinasadian/mini-wallet/pkg/database"
	pkgLogger "github.com/hosseinasadian/mini-wallet/pkg/logger"
	"github.com/hosseinasadian/mini-wallet/pkg/rabbitmq"
	amqp "github.com/rabbitmq/amqp091-go"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"
)

type Config struct {
	MainRepository               config.MySQL  `koanf:"mysql"`
	HTTPPort                     int           `koanf:"http_port"`
	Subscriber                   broker.Config `koanf:"subscriber"`
	HTTPShutDownCtxTimeout       time.Duration `koanf:"http_shut_down_timeout"`
	SubscriberShutdownCtxTimeout time.Duration `koanf:"subscriber_shutdown_timeout"`
}

type Application struct {
	config         Config
	httpServer     *http.Server
	userSubscriber broker.Subscriber
	logger         *pkgLogger.Logger
}

func Setup(config Config, conn *database.Database, logger *pkgLogger.Logger) Application {
	mainLogger := logger.With("layer", string(pkgLogger.LayerMain))

	repoLogger := logger.With("layer", string(pkgLogger.LayerRepository))
	walletRepo := walletRepository.NewRepository(conn.DB, repoLogger)

	serviceLogger := logger.With("layer", string(pkgLogger.LayerService))
	walletSvc := walletService.NewService(walletRepo, serviceLogger)

	httpLogger := logger.With("layer", string(pkgLogger.LayerHTTP))
	httpHandler := http.NewHandler(walletSvc, httpLogger)
	httpServer := http.NewServer(fmt.Sprintf(":%d", config.HTTPPort), httpHandler, httpLogger)

	// rabbitMq
	rbConn, err := rabbitmq.NewConnection(config.Subscriber.URL)
	if err != nil {
		mainLogger.Fatal("rabbitmq connection filed", "error", err)
	}
	//defer rbConn.Close()

	topology, err := rabbitmq.NewTopology(rbConn)
	if err != nil {
		mainLogger.Fatal("rabbitmq topology filed", "error", err)
	}
	//defer topology.Close()

	err = topology.DeclareTopic(rabbitmq.TopicTopologyConfig{
		EventName: "user",
		RetryTTL:  config.Subscriber.RetryTTL,
		Bindings: []rabbitmq.TopicBinding{
			{
				Queue:      "wallet-service",
				RoutingKey: "user.created",
			},
		},
	})
	if err != nil {
		mainLogger.Fatal("rabbitmq topology declared user topic failed", "error", err)
	}

	userSubscriber, err := rabbitmq.NewTopicSubscriber(
		rbConn,
		"user",
		"wallet-service",
		rabbitmq.SubscriberConfig{
			Workers:        config.Subscriber.Workers,
			MaxRetry:       config.Subscriber.MaxRetry,
			PrefetchCount:  config.Subscriber.PrefetchCount,
			HandlerTimeout: config.Subscriber.HandlerTimeout,

			OnPanic: func(rec any, msg amqp.Delivery) {
				mainLogger.Error("rabbitmq panic recovered", "err", rec)
			},

			OnDLQFail: func(msgID string, body []byte, err error) {
				mainLogger.Error("dlq publish failed", "error", err)
			},
		},
	)
	if err != nil {
		mainLogger.Fatal("rabbitmq subscriber topic failed", "error", err)
	}

	//defer func() {
	//	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	//	defer cancel()
	//
	//	if err := userSubscriber.Close(ctx); err != nil {
	//		log.Println("subscriber close error:", err)
	//	}
	//}()

	return Application{
		config:         config,
		httpServer:     httpServer,
		userSubscriber: userSubscriber,
		logger:         logger,
	}
}

func (app Application) Start() {
	logger := app.logger
	mainLogger := logger.With("layer", string(pkgLogger.LayerMain))
	var wg sync.WaitGroup

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)

	wg.Add(1)
	go func() {
		defer wg.Done()
		err := app.userSubscriber.Subscribe(func(
			ctx context.Context,
			msg broker.Message,
		) error {

			var evt struct {
				ID    string `json:"id"`
				Email string `json:"email"`
			}

			if err := json.Unmarshal(msg.Body, &evt); err != nil {
				return err
			}

			return nil
		})

		if err != nil {
			mainLogger.Error("user subscriber failed", "error", err)
			return
		}

		mainLogger.Info("user subscriber started")
	}()

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

	subCtx, subCancel := context.WithTimeout(context.Background(), app.config.SubscriberShutdownCtxTimeout)
	defer subCancel()

	if err := app.userSubscriber.Close(subCtx); err != nil {
		mainLogger.Warn("device publisher close error", "error", err)
	} else {
		mainLogger.Info("device publisher closed")
	}

	wg.Wait()
	mainLogger.Info("application stopped")
}
