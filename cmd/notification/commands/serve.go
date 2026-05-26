package commands

import (
	"context"
	notifApp "github.com/hosseinasadian/mini-wallet/internal/notification"
	pkgLogger "github.com/hosseinasadian/mini-wallet/pkg/logger"
	"github.com/hosseinasadian/mini-wallet/pkg/redis"
	"github.com/spf13/cobra"
)

var serveCmd = &cobra.Command{
	Use:   "serve",
	Short: "Serve User Authentication Service",
	Run: func(cmd *cobra.Command, args []string) {
		serve()
	},
}

func serve() {
	mainLogger := logger.With("layer", string(pkgLogger.LayerMain))

	redisAdapter, err := redis.New(context.Background(), notificationConfig.Redis)
	if err != nil {
		mainLogger.Fatal("failed to connect to redis", "error", err)
	} else {
		mainLogger.Info("Successfully connected to Redis")
	}

	app := notifApp.Setup(notificationConfig, redisAdapter, logger)
	app.Start()
}

func init() {
	RootCmd.AddCommand(serveCmd)
}
