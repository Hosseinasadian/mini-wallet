package main

import (
	"github.com/hosseinasadian/mini-wallet/cmd/wallet/commands"
	"log"
)

func main() {
	if err := commands.RootCmd.Execute(); err != nil {
		log.Fatal(err)
	}
}
