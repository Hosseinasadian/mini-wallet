package otel

type Config struct {
	ServiceName string `koanf:"service_name"`
	Environment string `koanf:"environment"`
	Endpoint    string `koanf:"endpoint"`
}
