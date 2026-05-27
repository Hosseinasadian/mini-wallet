package commands

import (
	"context"
	notifApp "github.com/hosseinasadian/mini-wallet/internal/notification"
	pkgLogger "github.com/hosseinasadian/mini-wallet/pkg/logger"
	pkgOtel "github.com/hosseinasadian/mini-wallet/pkg/otel"
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

	shutdownTracer, err := pkgOtel.InitTracer(notificationConfig.Otel)
	if err != nil {
		mainLogger.Fatal("failed to init tracer", "error", err)
	}
	defer func() {
		if err := shutdownTracer(context.Background()); err != nil {
			mainLogger.Warn("tracer shutdown failed", "error", err)
		}
	}()

	mp, err := pkgOtel.InitMetrics(notificationConfig.Otel)
	if err != nil {
		mainLogger.Fatal("failed to init metrics", "error", err)
	}

	redisAdapter, err := redis.New(context.Background(), notificationConfig.Redis)
	if err != nil {
		mainLogger.Fatal("failed to connect to redis", "error", err)
	} else {
		mainLogger.Info("Successfully connected to Redis")
	}

	app := notifApp.Setup(notificationConfig, redisAdapter, logger, mp)
	app.Start()
}

func init() {
	RootCmd.AddCommand(serveCmd)
}
