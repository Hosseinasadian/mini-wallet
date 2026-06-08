package grpc

import (
	"crypto/tls"
	"crypto/x509"
	"os"
	"time"

	"github.com/hosseinasadian/mini-wallet/pkg/grpc/interceptors"
	"github.com/hosseinasadian/mini-wallet/pkg/richerror"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/keepalive"
)

type Config struct {
	Port               int           `koanf:"port"`
	NetworkType        string        `koanf:"type"`
	ShutDownCtxTimeout time.Duration `koanf:"shutdown_context_timeout"`
	TLSEnabled         bool          `koanf:"tls_enabled"`
	TLSCertPath        string        `koanf:"tls_cert_path"` // Path to server certificate
	TLSKeyPath         string        `koanf:"tls_key_path"`  // Path to server private key
}

type RPCServer struct {
	Config Config
	Server *grpc.Server
}

func New(cfg Config, interceptors *interceptors.Interceptors) (RPCServer, error) {
	const op = "grpc.New"

	var opts []grpc.ServerOption

	// Add interceptors (MUST be before other options)
	opts = append(opts,
		grpc.ChainUnaryInterceptor(
			interceptors.UnaryServerInterceptor(),
		),
		grpc.ChainStreamInterceptor(
			interceptors.StreamServerInterceptor(),
		),
	)

	// Add TLS if enabled
	if cfg.TLSEnabled {
		// Load TLS credentials
		creeds, err := loadServerTLSCredentials(cfg)
		if err != nil {
			return RPCServer{}, richerror.New(op).
				WithWrapper(err).
				WithMessage("failed to load TLS credentials").
				WithKind(richerror.KindInternal)
		}
		opts = append(opts, grpc.Creds(creeds))
	}

	// Add keepalive params
	opts = append(opts,
		grpc.KeepaliveParams(keepalive.ServerParameters{
			MaxConnectionIdle: 15 * time.Minute,
			MaxConnectionAge:  30 * time.Minute,
			Time:              5 * time.Minute,
			Timeout:           1 * time.Minute,
		}),
		grpc.KeepaliveEnforcementPolicy(keepalive.EnforcementPolicy{
			MinTime:             20 * time.Second, // Reduced from 1m to allow client 30s pings
			PermitWithoutStream: true,
		}),
	)

	grpcServer := grpc.NewServer(opts...)

	return RPCServer{
		Server: grpcServer,
		Config: cfg,
	}, nil
}

func loadServerTLSCredentials(cfg Config) (credentials.TransportCredentials, error) {
	const op = "grpc.loadServerTLSCredentials"

	// Validate certificate paths
	if cfg.TLSCertPath == "" || cfg.TLSKeyPath == "" {
		return nil, richerror.New(op).
			WithMessage("TLS enabled but certificate or key path not provided").
			WithKind(richerror.KindInternal)
	}

	// Check if files exist
	if _, err := os.Stat(cfg.TLSCertPath); os.IsNotExist(err) {
		return nil, richerror.New(op).
			WithWrapper(err).
			WithMessage("certificate file not found").
			WithKind(richerror.KindInternal)
	}
	if _, err := os.Stat(cfg.TLSKeyPath); os.IsNotExist(err) {
		return nil, richerror.New(op).
			WithWrapper(err).
			WithMessage("key file not found").
			WithKind(richerror.KindInternal)
	}

	// Load server certificate and key
	cert, err := tls.LoadX509KeyPair(cfg.TLSCertPath, cfg.TLSKeyPath)
	if err != nil {
		return nil, richerror.New(op).
			WithWrapper(err).
			WithMessage("failed to load key pair").
			WithKind(richerror.KindInternal)
	}

	// Validate certificate at startup
	if len(cert.Certificate) == 0 {
		return nil, richerror.New(op).
			WithMessage("certificate chain is empty").
			WithKind(richerror.KindInternal)
	}

	x509Cert, err := x509.ParseCertificate(cert.Certificate[0])
	if err != nil {
		return nil, richerror.New(op).
			WithWrapper(err).
			WithMessage("failed to parse certificate").
			WithKind(richerror.KindInternal)
	}

	// Check certificate validity period
	now := time.Now()
	if now.Before(x509Cert.NotBefore) {
		return nil, richerror.New(op).
			WithMessage("certificate not yet valid").
			WithKind(richerror.KindInternal)
	}

	if now.After(x509Cert.NotAfter) {
		return nil, richerror.New(op).
			WithMessage("certificate has expired").
			WithKind(richerror.KindInternal)
	}

	// Create TLS config
	tlsConfig := &tls.Config{
		Certificates: []tls.Certificate{cert},
		ClientAuth:   tls.NoClientCert, // Can be changed to tls.RequireAndVerifyClientCert for mTLS
		MinVersion:   tls.VersionTLS12, // Enforce minimum TLS version
	}

	return credentials.NewTLS(tlsConfig), nil
}

func (s RPCServer) Stop() {
	s.Server.GracefulStop()
}
