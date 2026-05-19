package config

type Swagger struct {
	Host    string `koanf:"host"`
	Schemes string `koanf:"schemes"`
}
