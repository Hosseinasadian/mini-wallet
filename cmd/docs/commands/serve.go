package commands

import (
	"context"
	docsApp "github.com/hosseinasadian/mini-wallet/internal/docs"
	pkgLogger "github.com/hosseinasadian/mini-wallet/pkg/logger"
	pkgOtel "github.com/hosseinasadian/mini-wallet/pkg/otel"
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
	mainLogger := logger.With("layer", string(pkgLogger.LayerMain))

	shutdownTracer, err := pkgOtel.InitTracer(docsConfig.Otel)
	if err != nil {
		mainLogger.Fatal("failed to init tracer", "error", err)
	}
	defer func() {
		if err := shutdownTracer(context.Background()); err != nil {
			mainLogger.Warn("tracer shutdown failed", "error", err)
		}
	}()

	mp, err := pkgOtel.InitMetrics(docsConfig.Otel)
	if err != nil {
		mainLogger.Fatal("failed to init metrics", "error", err)
	}

	app := docsApp.Setup(docsConfig, logger, mp)
	app.Start()
}

func init() {
	RootCmd.AddCommand(serveCmd)
}
