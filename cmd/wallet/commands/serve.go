package commands

import (
	"errors"
	"fmt"
	"github.com/golang-migrate/migrate/v4"
	walletApp "github.com/hosseinasadian/mini-wallet/internal/wallet"
	"github.com/jmoiron/sqlx"
	"github.com/spf13/cobra"
	"log"
)

var serveCmd = &cobra.Command{
	Use:   "serve",
	Short: "Serve Wallet Service",
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

	dsn := fmt.Sprintf("%s:%s@(%s:%d)/%s?parseTime=true", walletConfig.MainRepository.Username, walletConfig.MainRepository.Password, walletConfig.MainRepository.Host, walletConfig.MainRepository.Port, walletConfig.MainRepository.Database)
	db, err := sqlx.Connect("mysql", dsn)
	if err != nil {
		fmt.Printf("Connect config failed, err:%v\n", err)
		log.Fatal("Connect config failed")
	}

	app := walletApp.Setup(walletConfig, db)
	app.Start()
}

func init() {
	serveCmd.Flags().BoolVar(&migrateUp, "migrate-up", false, "migrate up")
	RootCmd.AddCommand(serveCmd)
}
