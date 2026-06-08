package grpc

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"os"
	"time"

	"github.com/hosseinasadian/mini-wallet/pkg/grpc/interceptors"
	"github.com/hosseinasadian/mini-wallet/pkg/richerror"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/keepalive"
)

type Client struct {
	Host               string `koanf:"host"`
	Port               int    `koanf:"port"`
	TLSEnabled         bool   `koanf:"tls_enabled"`
	TLSCertPath        string `koanf:"tls_cert_path"`        // Path to CA certificate
	InsecureSkipVerify bool   `koanf:"insecure_skip_verify"` // Only for development
}

func NewClient(cfg Client, interceptors *interceptors.Interceptors) (*grpc.ClientConn, error) {
	const op = "grpc.NewConnection"

	address := fmt.Sprintf("%s:%d", cfg.Host, cfg.Port)

	var opts []grpc.DialOption

	opts = append(opts,
		grpc.WithChainUnaryInterceptor(
			interceptors.UnaryClientInterceptor(),
		),
		grpc.WithChainStreamInterceptor(
			interceptors.StreamClientInterceptor(),
		),
	)

	// Add TLS or insecure credentials
	if cfg.TLSEnabled {
		// Load TLS credentials
		creeds, err := loadTLSCredentials(cfg)
		if err != nil {
			return nil, richerror.New(op).
				WithWrapper(err).
				WithMessage("failed to load TLS credentials").
				WithKind(richerror.KindInternal)
		}
		opts = append(opts, grpc.WithTransportCredentials(creeds))
	} else {
		// Insecure mode (development only)
		opts = append(opts, grpc.WithTransportCredentials(insecure.NewCredentials()))
	}

	// Add keepalive params
	opts = append(opts,
		grpc.WithKeepaliveParams(keepalive.ClientParameters{
			Time:                10 * time.Second,
			Timeout:             2 * time.Second,
			PermitWithoutStream: true,
		}),
	)

	// Use modern grpc.NewClient (non-blocking, lazy connection)
	conn, err := grpc.NewClient(address, opts...)
	if err != nil {
		return nil, richerror.New(op).
			WithWrapper(err).
			WithMessage("failed to create gRPC client").
			WithKind(richerror.KindInternal)
	}

	return conn, nil
}

func loadTLSCredentials(cfg Client) (credentials.TransportCredentials, error) {
	const op = "grpc.loadTLSCredentials"

	// Create TLS config
	tlsConfig := &tls.Config{
		ServerName:         cfg.Host,
		InsecureSkipVerify: cfg.InsecureSkipVerify, // Should be false in production
	}

	// Load CA certificate if provided
	if cfg.TLSCertPath != "" {
		caCert, err := os.ReadFile(cfg.TLSCertPath)
		if err != nil {
			return nil, richerror.New(op).
				WithWrapper(err).
				WithMessage("failed to read CA certificate").
				WithKind(richerror.KindInternal)
		}

		certPool := x509.NewCertPool()
		if !certPool.AppendCertsFromPEM(caCert) {
			return nil, richerror.New(op).
				WithMessage("failed to add CA certificate to pool").
				WithKind(richerror.KindInternal)
		}

		tlsConfig.RootCAs = certPool
	}

	return credentials.NewTLS(tlsConfig), nil
}
