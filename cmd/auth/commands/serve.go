package commands

import (
	"errors"
	"fmt"
	"github.com/golang-migrate/migrate/v4"
	authApp "github.com/hosseinasadian/mini-wallet/internal/auth"
	"github.com/jmoiron/sqlx"
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

var migrateUp bool

func serve() {
	if migrateUp {
		fmt.Println("Run migration up...")
		m := migrateDatabase()
		if err := m.Up(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
			log.Fatalf("Up migration failed , err:%v\n", err)
		}
		fmt.Println("Run migration up completed")
	}

	dsn := fmt.Sprintf("%s:%s@(%s:%d)/%s?parseTime=true", authConfig.MainRepository.Username, authConfig.MainRepository.Password, authConfig.MainRepository.Host, authConfig.MainRepository.Port, authConfig.MainRepository.Database)
	db, err := sqlx.Connect("mysql", dsn)
	if err != nil {
		fmt.Printf("Connect config failed, err:%v\n", err)
		log.Fatal("Connect config failed")
	}

	app := authApp.Setup(authConfig, db)
	app.Start()
}

func init() {
	serveCmd.Flags().BoolVar(&migrateUp, "migrate-up", false, "migrate up")
	RootCmd.AddCommand(serveCmd)
}
