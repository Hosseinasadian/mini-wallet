package commands

import (
	"errors"
	"github.com/golang-migrate/migrate/v4"
	authApp "github.com/hosseinasadian/mini-wallet/internal/auth"
	"github.com/hosseinasadian/mini-wallet/pkg/database"
	pkgLogger "github.com/hosseinasadian/mini-wallet/pkg/logger"
	"github.com/spf13/cobra"
)

var serveCmd = &cobra.Command{
	Use:   "serve",
	Short: "Serve User Authentication Service",
	Run: func(cmd *cobra.Command, args []string) {
		serve()
	},
}

var migrateUp bool

func serve() {
	mainLogger := logger.With("layer", string(pkgLogger.LayerMain))

	dbLogger := logger.With("layer", string(pkgLogger.LayerMysql))
	err := database.SetLogger(dbLogger)
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

	conn, err := database.Connect(&authConfig.MainRepository)
	if err != nil {
		mainLogger.Fatal("database connection failed", "error", err)
	}
	defer database.Close(conn.DB)

	app := authApp.Setup(authConfig, conn, logger)
	app.Start()
}

func init() {
	serveCmd.Flags().BoolVar(&migrateUp, "migrate-up", false, "migrate up")
	RootCmd.AddCommand(serveCmd)
}
