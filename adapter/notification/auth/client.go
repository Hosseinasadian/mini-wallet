package auth

import (
	"context"
	authpb "github.com/hosseinasadian/mini-wallet/gen/go/auth/v1"
	"github.com/hosseinasadian/mini-wallet/internal/notification/subscriber"
	"google.golang.org/grpc"
)

type Client struct {
	Conn *grpc.ClientConn
}

func New(conn *grpc.ClientConn) *Client {
	return &Client{
		Conn: conn,
	}
}

func (c Client) GetUserActiveSessions(ctx context.Context, userId int64) ([]subscriber.SessionItem, error) {
	client := authpb.NewAuthServiceClient(c.Conn)

	res, err := client.GetUserActiveSessions(ctx, &authpb.GetUserActiveSessionsRequest{
		UserId: userId,
	})

	if err != nil {
		return nil, err
	}

	sessions := make([]subscriber.SessionItem, 0, len(res.Sessions))

	for _, session := range res.Sessions {
		sessions = append(sessions, subscriber.SessionItem{
			SessionID: session.SessionId,
			PushToken: session.PushToken,
		})
	}

	return sessions, nil
}
