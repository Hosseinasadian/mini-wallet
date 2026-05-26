package commands

import (
	"errors"
	"github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/database/mysql"
	_ "github.com/golang-migrate/migrate/v4/database/mysql"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	"github.com/hosseinasadian/mini-wallet/pkg/database"
	pkgLogger "github.com/hosseinasadian/mini-wallet/pkg/logger"
	"github.com/spf13/cobra"
)

var migrateCmd = &cobra.Command{
	Use:   "migrate",
	Short: "Migrate Wallet Service",
}

var migrateUpCmd = &cobra.Command{
	Use:   "up",
	Short: "Run migration up",
	Run: func(cmd *cobra.Command, args []string) {
		mainLogger := logger.With("layer", string(pkgLogger.LayerMain))

		mainLogger.Info("running migration up")
		m := migrateDatabase()
		if err := m.Up(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
			mainLogger.Fatal("migration up failed", "error", err)
		}
		mainLogger.Info("migration up completed")
	},
}

var migrateDownCmd = &cobra.Command{
	Use:   "down",
	Short: "Run migration down",
	Run: func(cmd *cobra.Command, args []string) {
		mainLogger := logger.With("layer", string(pkgLogger.LayerMain))

		mainLogger.Info("running migration down")
		m := migrateDatabase()
		if err := m.Down(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
			mainLogger.Fatal("migration down failed", "error", err)
		}
		mainLogger.Info("migration down completed")
	},
}

func init() {
	migrateCmd.AddCommand(migrateUpCmd, migrateDownCmd)
	RootCmd.AddCommand(migrateCmd)
}

func migrateDatabase() *migrate.Migrate {
	mainLogger := logger.With("layer", string(pkgLogger.LayerMain))

	dbLogger := logger.With("layer", string(pkgLogger.LayerMysql))
	err := database.SetLogger(dbLogger)
	if err != nil {
		mainLogger.Fatal("failed to set logger", "error", err)
	}

	conn, err := database.Connect(&walletConfig.MainRepository)
	if err != nil {
		mainLogger.Fatal("database connection failed", "error", err)
	}

	driver, err := mysql.WithInstance(conn.DB.DB, &mysql.Config{MigrationsTable: "wallet_schema_migrations"})
	m, err := migrate.NewWithDatabaseInstance("file://internal/wallet/repository/migrations", "mysql", driver)
	if err != nil {
		logger.Fatal("failed to migrate", "error", err)
	}

	return m
}
