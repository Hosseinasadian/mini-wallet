package grpc

import (
	"context"
	authpb "github.com/hosseinasadian/mini-wallet/gen/go/auth/v1"
	authService "github.com/hosseinasadian/mini-wallet/internal/auth/service/auth"
)

type Handler struct {
	authpb.UnimplementedAuthServiceServer
	authSvc *authService.Service
}

func NewHandler(authSvc *authService.Service) Handler {
	return Handler{
		UnimplementedAuthServiceServer: authpb.UnimplementedAuthServiceServer{},
		authSvc:                        authSvc,
	}
}

func (h Handler) GetUserActiveSessions(ctx context.Context, req *authpb.GetUserActiveSessionsRequest) (*authpb.GetUserActiveSessionsResponse, error) {
	sessions, err := h.authSvc.GetActiveSessions(ctx, req.UserId)
	if err != nil {
		return nil, err
	}

	sessionsPb := make([]*authpb.Session, 0, len(sessions))

	for _, session := range sessions {
		pushToken := ""
		if session.PushToken != nil && *session.PushToken != "" {
			pushToken = *session.PushToken
		}

		sessionsPb = append(sessionsPb, &authpb.Session{
			UserId:    req.UserId,
			SessionId: session.SessionID,
			PushToken: pushToken,
		})
	}

	return &authpb.GetUserActiveSessionsResponse{
		Sessions: sessionsPb,
	}, nil
}
