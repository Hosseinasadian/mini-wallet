package commands

import (
	docsApp "github.com/hosseinasadian/mini-wallet/internal/docs"
	"github.com/spf13/cobra"
)

var serveCmd = &cobra.Command{
	Use:   "serve",
	Short: "Serve Wallet Documentation",
	Run: func(cmd *cobra.Command, args []string) {
		serve()
	},
}

func serve() {
	app := docsApp.Setup(docsConfig, logger)
	app.Start()
}

func init() {
	RootCmd.AddCommand(serveCmd)
}
