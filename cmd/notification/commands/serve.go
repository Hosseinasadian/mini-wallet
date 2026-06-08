package commands

import (
	"context"
	"errors"
	"github.com/golang-migrate/migrate/v4"
	notifApp "github.com/hosseinasadian/mini-wallet/internal/notification"
	"github.com/hosseinasadian/mini-wallet/pkg/database"
	pkgLogger "github.com/hosseinasadian/mini-wallet/pkg/logger"
	pkgOtel "github.com/hosseinasadian/mini-wallet/pkg/otel"
	"github.com/hosseinasadian/mini-wallet/pkg/redis"
	"github.com/spf13/cobra"
)

var serveCmd = &cobra.Command{
	Use:   "serve",
	Short: "Serve Notification Service",
	Run: func(cmd *cobra.Command, args []string) {
		serve()
	},
}

var migrateUp bool

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

	dbLogger := logger.With("layer", string(pkgLogger.LayerMysql))
	err = database.SetLogger(dbLogger)
	if err != nil {
		mainLogger.Fatal("failed to set logger", "error", err)
	}

	if migrateUp {
		mainLogger.Info("running migration up")
		m := migrateDatabase()
		if err := m.Up(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
			mainLogger.Fatal("migration up failed", "error", err)
		}
		mainLogger.Info("migration up completed")
	}

	conn, err := database.Connect(&notificationConfig.MainRepository)
	if err != nil {
		mainLogger.Fatal("database connection failed", "error", err)
	}
	defer database.Close(conn.DB)

	redisAdapter, err := redis.New(context.Background(), notificationConfig.Redis)
	if err != nil {
		mainLogger.Fatal("failed to connect to redis", "error", err)
	} else {
		mainLogger.Info("Successfully connected to Redis")
	}

	app := notifApp.Setup(notificationConfig, conn, redisAdapter, logger, mp)
	app.Start()
}

func init() {
	serveCmd.Flags().BoolVar(&migrateUp, "migrate-up", false, "migrate up")
	RootCmd.AddCommand(serveCmd)
}
