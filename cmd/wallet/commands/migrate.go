package commands

import (
	"errors"
	"fmt"
	"github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/database/mysql"
	_ "github.com/golang-migrate/migrate/v4/database/mysql"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	"github.com/jmoiron/sqlx"
	"github.com/spf13/cobra"
	"log"
)

var migrateCmd = &cobra.Command{
	Use:   "migrate",
	Short: "Migrate Wallet Service",
}

var migrateUpCmd = &cobra.Command{
	Use:   "up",
	Short: "Run migration up",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println("Run migration up...")
		m := migrateDatabase()
		if err := m.Up(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
			log.Fatalf("Up migration failed , err:%v\n", err)
		}
		fmt.Println("Run migration up completed")
	},
}

var migrateDownCmd = &cobra.Command{
	Use:   "down",
	Short: "Run migration down",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println("Run migration down...")
		m := migrateDatabase()
		if err := m.Down(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
			log.Fatalf("Up migration failed  , err:%v\n", err)
		}
		fmt.Println("Run migration down completed")
	},
}

func init() {
	migrateCmd.AddCommand(migrateUpCmd, migrateDownCmd)
	RootCmd.AddCommand(migrateCmd)
}

func migrateDatabase() *migrate.Migrate {
	var err error
	var db *sqlx.DB

	dsn := fmt.Sprintf("%s:%s@tcp(%s:%d)/%s?parseTime=true", walletConfig.MainRepository.Username, walletConfig.MainRepository.Password, walletConfig.MainRepository.Host, walletConfig.MainRepository.Port, walletConfig.MainRepository.Database)
	db, err = sqlx.Connect("mysql", dsn)
	if err != nil {
		log.Fatal("Connect config failed")
	}

	driver, err := mysql.WithInstance(db.DB, &mysql.Config{MigrationsTable: "wallet_schema_migrations"})
	m, err := migrate.NewWithDatabaseInstance("file://internal/wallet/repository/migrations", "mysql", driver)
	if err != nil {
		log.Fatal("Migrate migration failed")
	}

	return m
}
