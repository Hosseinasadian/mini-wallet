package commands

import (
	"github.com/hosseinasadian/mini-wallet/internal/auth"
	pkgLogger "github.com/hosseinasadian/mini-wallet/pkg/logger"
	"github.com/knadh/koanf/parsers/yaml"
	"github.com/knadh/koanf/providers/env/v2"
	"github.com/knadh/koanf/providers/file"
	"github.com/knadh/koanf/v2"
	"github.com/spf13/cobra"
	"os"
	"path/filepath"
	"strings"
)

var RootCmd = &cobra.Command{
	Use:   "Auth Service",
	Short: "User Authentication Service",
	Long:  "Responsible for User Login|Register|Logout and RefreshToken|VerifyToken",
}

var authConfig auth.Config
var logger *pkgLogger.Logger

func ReadAuthConfig() {
	mainLogger := logger.With("layer", string(pkgLogger.LayerMain))

	dir, err := os.Getwd()
	if err != nil {
		mainLogger.Fatal("unable to get working directory", "error", err)
	}

	environment := os.Getenv("ENV")
	if environment == "" {
		environment = "development"
	}

	configPath := filepath.Join(dir, "deployment", "auth", environment, "config.yaml")

	k := koanf.New(".")
	if err := k.Load(file.Provider(configPath), yaml.Parser()); err != nil {
		mainLogger.Fatal("unable to load config", "error", err)
	}

	if err := k.Load(env.Provider(".", env.Opt{
		Prefix: "AUTH_",
		TransformFunc: func(k, v string) (string, any) {
			k = strings.TrimPrefix(k, "AUTH_")
			k = strings.ReplaceAll(k, "_", ".")
			k = strings.ReplaceAll(k, "..", "_")
			k = strings.ToLower(k)

			if strings.Contains(v, " ") {
				return k, strings.Split(v, " ")
			}

			return k, v
		},
	}), nil); err != nil {
		mainLogger.Fatal("unable to read config", "error", err)
	}

	if err := k.Unmarshal("", &authConfig); err != nil {
		mainLogger.Fatal("unable to unmarshal config", "error", err)
	}
}

func InitLogger() {
	logger = pkgLogger.New().With(
		"app", string(pkgLogger.AppAuth),
	)
}

func init() {
	InitLogger()
	ReadAuthConfig()
}
