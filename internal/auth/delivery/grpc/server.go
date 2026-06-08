package grpc

import (
	"fmt"
	authpb "github.com/hosseinasadian/mini-wallet/gen/go/auth/v1"
	"github.com/hosseinasadian/mini-wallet/pkg/grpc"
	"net"
)

type Server struct {
	server  grpc.RPCServer
	handler Handler
}

func New(server grpc.RPCServer, handler Handler) Server {
	return Server{
		server:  server,
		handler: handler,
	}
}

func (s Server) Serve() error {
	listener, err := net.Listen(s.server.Config.NetworkType, fmt.Sprintf(":%d", s.server.Config.Port))
	if err != nil {
		return err
	}

	authpb.RegisterAuthServiceServer(s.server.Server, s.handler)
	if err := s.server.Server.Serve(listener); err != nil {
		return err
	}
	return nil
}

func (s Server) Stop() {
	s.server.Stop()
}
