package commands

import (
	"github.com/hosseinasadian/mini-wallet/internal/wallet"
	"github.com/knadh/koanf/parsers/yaml"
	"github.com/knadh/koanf/providers/env/v2"
	"github.com/knadh/koanf/providers/file"
	"github.com/knadh/koanf/v2"
	"github.com/spf13/cobra"
	"log"
	"os"
	"path/filepath"
	"strings"
)

var RootCmd = &cobra.Command{
	Use:   "Wallet Service",
	Short: "Wallet Service",
	Long:  "Responsible for User wallet management and transfers",
}

var walletConfig wallet.Config

func ReadWalletConfig() {
	dir, err := os.Getwd()
	if err != nil {
		log.Fatalf("could not get working directory: %v", err)
	}

	environment := os.Getenv("ENV")
	if environment == "" {
		environment = "development"
	}

	configPath := filepath.Join(dir, "deployment", "wallet", environment, "config.yaml")

	k := koanf.New(".")
	if err := k.Load(file.Provider(configPath), yaml.Parser()); err != nil {
		log.Fatalf("could not load config: %v", err)
	}

	if err := k.Load(env.Provider(".", env.Opt{
		Prefix: "WALLET_",
		TransformFunc: func(k, v string) (string, any) {
			k = strings.TrimPrefix(k, "WALLET_")
			k = strings.ReplaceAll(k, "_", ".")
			k = strings.ReplaceAll(k, "..", "_")
			k = strings.ToLower(k)

			if strings.Contains(v, " ") {
				return k, strings.Split(v, " ")
			}

			return k, v
		},
	}), nil); err != nil {
		log.Fatalf("could not read config: %v", err)
	}

	if err := k.Unmarshal("", &walletConfig); err != nil {
		log.Fatal(err)
	}
}

func init() {
	ReadWalletConfig()
}
