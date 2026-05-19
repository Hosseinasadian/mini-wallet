package notification

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/hosseinasadian/mini-wallet/internal/notification/delivery/http"
	"github.com/hosseinasadian/mini-wallet/internal/notification/service/notification"
	"github.com/hosseinasadian/mini-wallet/pkg/broker"
	"github.com/hosseinasadian/mini-wallet/pkg/hub"
	"github.com/hosseinasadian/mini-wallet/pkg/one_signal"
	"github.com/hosseinasadian/mini-wallet/pkg/rabbitmq"
	"github.com/hosseinasadian/mini-wallet/pkg/redis"
	"github.com/hosseinasadian/mini-wallet/pkg/sender"
	amqp "github.com/rabbitmq/amqp091-go"
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
}

func Setup(config Config, redisAdapter *redis.Redis) Application {

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

	err = topology.DeclareDirect(rabbitmq.DirectTopologyConfig{
		EventName: "notification",
		RetryTTL:  config.Subscriber.RetryTTL,
	})
	if err != nil {
		log.Fatal(err)
	}

	notificationSvc := notification.NewService(config.NotificationService, redisAdapter)
	notificationHub := hub.NewHub(redisAdapter)

	repo := one_signal.FakeRepo{}
	senderOneSignal := one_signal.NewOneSignalSender(config.Sender.OneSignal, &repo)

	httpHandler := http.NewHandler(notificationSvc, notificationHub)
	httpServer := http.NewServer(fmt.Sprintf(":%d", config.HTTPPort), httpHandler, config.JWTSecret)

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
		log.Fatal(err)
	}

	return Application{config: config, httpServer: httpServer, notificationSubscriber: notificationSubscriber, hub: notificationHub, sender: senderOneSignal}
}

func (app Application) Start() {
	var wg sync.WaitGroup

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)

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

			log.Println("RECEIVED:", evt.Message, evt.UserID)

			userIdStr := fmt.Sprintf("%d", evt.UserID)
			if app.hub.IsOnline(userIdStr) {
				app.hub.Publish(userIdStr, evt.Message)
			} else {
				if err := app.sender.Send(ctx, userIdStr, "new notification", evt.Message); err != nil {
					log.Printf("push notification error: %v", err)
				}
			}

			return nil
		})

		if err != nil {
			log.Printf("Subscriber error: %v", err)
			return
		}

		log.Println("notification consumer started")
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		app.httpServer.Run()
	}()

	<-stop
	log.Println("⚠️ Received shutdown signal, initiating graceful shutdown...")

	httpCtx, httpCancel := context.WithTimeout(ctx, app.config.HTTPShutDownCtxTimeout)
	defer httpCancel()

	if err := app.httpServer.Stop(httpCtx); err != nil {
		log.Printf("HTTP server shutdown error: %v", err)
	}

	subCtx, subCancel := context.WithTimeout(ctx, app.config.SubscriberShutdownCtxTimeout)
	defer subCancel()

	if err := app.notificationSubscriber.Close(subCtx); err != nil {
		log.Println(err)
	} else {
		log.Println("notification subscriber closed")
	}

	app.hub.Close()

	wg.Wait()
	log.Println("notification app stopped")
}
