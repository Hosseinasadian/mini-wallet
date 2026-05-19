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
	"github.com/hosseinasadian/mini-wallet/pkg/rabbitmq"
	"github.com/jmoiron/sqlx"
	amqp "github.com/rabbitmq/amqp091-go"
	"log"
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
}

func Setup(config Config, db *sqlx.DB) Application {
	walletRepo := walletRepository.NewRepository(db)
	walletSvc := walletService.NewService(walletRepo)
	httpHandler := http.NewHandler(walletSvc)
	httpServer := http.NewServer(fmt.Sprintf(":%d", config.HTTPPort), httpHandler)

	// rabbitMq
	rbConn, err := rabbitmq.NewConnection(config.Subscriber.URL)
	if err != nil {
		log.Fatal(err)
	}
	//defer rbConn.Close()

	topology, err := rabbitmq.NewTopology(rbConn)
	if err != nil {
		log.Fatal(err)
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
		log.Fatal(err)
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
				log.Println("handler panic:", rec)
			},

			OnDLQFail: func(msgID string, body []byte, err error) {
				log.Println("dlq publish failed:", err)
			},
		},
	)
	if err != nil {
		log.Fatal(err)
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
	}
}

func (app Application) Start() {
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

			log.Println("RECEIVED:", evt.ID, evt.Email)

			return nil
		})

		if err != nil {
			log.Printf("Subscriber error: %v", err)
			return
		}

		log.Println("wallet consumer started")
	}()

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

	subCtx, subCancel := context.WithTimeout(context.Background(), app.config.SubscriberShutdownCtxTimeout)
	defer subCancel()

	if err := app.userSubscriber.Close(subCtx); err != nil {
		log.Println(err)
	} else {
		log.Println("user subscriber closed")
	}

	wg.Wait()
	log.Println("wallet app stopped")
}
