package commands

import (
	"context"
	notifApp "github.com/hosseinasadian/mini-wallet/internal/notification"
	"github.com/hosseinasadian/mini-wallet/pkg/redis"
	"github.com/spf13/cobra"
	"log"
)

var serveCmd = &cobra.Command{
	Use:   "serve",
	Short: "Serve User Authentication Service",
	Run: func(cmd *cobra.Command, args []string) {
		serve()
	},
}

func serve() {
	redisAdapter, err := redis.New(context.Background(), notificationConfig.Redis)
	if err != nil {
		log.Fatalf("failed to connect to redis: %v", err)
	} else {
		log.Printf("Successfully connected to Redis: %v", notificationConfig.Redis)
	}

	app := notifApp.Setup(notificationConfig, redisAdapter)
	app.Start()
}

func init() {
	RootCmd.AddCommand(serveCmd)
}
