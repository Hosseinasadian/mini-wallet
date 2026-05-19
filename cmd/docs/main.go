package main

import (
	"github.com/hosseinasadian/mini-wallet/cmd/docs/commands"
	"os"
)

func main() {
	if err := commands.RootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}
