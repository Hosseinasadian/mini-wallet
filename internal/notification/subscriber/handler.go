package subscriber

import (
	"context"
	"errors"
	"fmt"
	"github.com/google/uuid"
	authpb "github.com/hosseinasadian/mini-wallet/gen/go/auth/v1"
	commonpb "github.com/hosseinasadian/mini-wallet/gen/go/common/v1"
	notificationpb "github.com/hosseinasadian/mini-wallet/gen/go/notification/v1"
	eventService "github.com/hosseinasadian/mini-wallet/internal/notification/service/event"
	"github.com/hosseinasadian/mini-wallet/internal/notification/service/outbox"
	outboxService "github.com/hosseinasadian/mini-wallet/internal/notification/service/outbox"
	"github.com/hosseinasadian/mini-wallet/pkg/broker"
	"github.com/hosseinasadian/mini-wallet/pkg/event"
	"github.com/hosseinasadian/mini-wallet/pkg/hub"
	pkgLogger "github.com/hosseinasadian/mini-wallet/pkg/logger"
	"github.com/hosseinasadian/mini-wallet/pkg/richerror"
	"google.golang.org/protobuf/proto"
	"time"
)

type SessionItem struct {
	SessionID string
	PushToken string
}

type Sender interface {
	Send(ctx context.Context, userID string, title, message string) error
}

type AuthClient interface {
	GetUserActiveSessions(ctx context.Context, userId int64) ([]SessionItem, error)
}

type Handler struct {
	hub        *hub.Hub
	authClient AuthClient
	sender     Sender
	outboxSvc  *outboxService.Service
	eventSvc   *eventService.Service
	logger     *pkgLogger.Logger
}

func New(hub *hub.Hub, authClient AuthClient, sender Sender, outboxSvc *outboxService.Service, eventSvc *eventService.Service, logger *pkgLogger.Logger) *Handler {
	return &Handler{
		hub:        hub,
		authClient: authClient,
		sender:     sender,
		outboxSvc:  outboxSvc,
		eventSvc:   eventSvc,
		logger:     logger,
	}
}

func (h *Handler) Handle(ctx context.Context, msg broker.Message) error {
	err := h.eventSvc.RunProcess(ctx, msg.ID, msg.Body)
	if err != nil {
		var re *richerror.RichError
		if errors.As(err, &re) && re.Kind() == richerror.KindConflict {
			h.logger.Info("duplicate message skipped", "msg_id", msg.ID)
			return nil
		}
		return err
	}

	var ev commonpb.EventEnvelope

	if err := proto.Unmarshal(msg.Body, &ev); err != nil {
		h.logger.Error("failed to unmarshal message", "error", err)
		return err
	}

	if err := h.handleEvent(ctx, &ev); err != nil {
		err = h.eventSvc.MarkFailed(ctx, msg.ID)
		if err != nil {
			h.logger.Warn("failed to mark failed event", "error", err, "msg_id", msg.ID)
		}
		return err
	}

	err = h.eventSvc.MarkSent(ctx, msg.ID)
	if err != nil {
		h.logger.Warn("failed to mark sent event", "error", err, "msg_id", msg.ID)
	}

	return nil
}

func (h *Handler) handleEvent(ctx context.Context, ev *commonpb.EventEnvelope) error {
	switch ev.EventType {
	case string(event.TypeAuthNewSessionLoggedIn):
		return h.handleNewSession(ctx, ev)
	case string(event.TypeNotificationPush):
		return h.handlePushNotification(ctx, ev)
	default:
		return nil
	}
}

func (h *Handler) handleNewSession(ctx context.Context, ev *commonpb.EventEnvelope) error {
	var newSessionEv authpb.NewSessionLoggedIn
	err := ev.Payload.UnmarshalTo(&newSessionEv)
	if err != nil {
		h.logger.Error("failed to unmarshal login event", "error", err)
		return err
	} else {
		h.logger.Info("login new session", "session", newSessionEv.Id, "userId", newSessionEv.UserId)

		sessions, err := h.authClient.GetUserActiveSessions(ctx, newSessionEv.UserId)
		if err != nil {
			h.logger.Error("failed to get active sessions", "error", err)
			return err
		}

		if len(sessions) > 0 {
			sessionEvents := make([]*outbox.OutboxEvent, 0, len(sessions))

			for _, session := range sessions {
				if session.SessionID == newSessionEv.Id {
					continue
				}

				notifEvent := &notificationpb.NotificationEvent{
					EventId:   uuid.New().String(),
					EventType: notificationpb.EventType_EVENT_TYPE_ALERT_SESSION,
					Payload: &notificationpb.NotificationEvent_SessionPayload{
						SessionPayload: &notificationpb.SessionPayload{
							UserId:    newSessionEv.UserId,
							SessionId: session.SessionID,
							PushToken: session.PushToken,
						},
					},
				}
				notifToCommon, err := event.New(notifEvent, event.TypeNotificationPush)

				if err != nil {
					h.logger.Error("failed to create new logged event", "error", err)
					return err
				}

				notifEventBody, err := proto.Marshal(notifToCommon)
				if err != nil {
					h.logger.Warn("event marshaling failed", "error", err)
					return err
				}

				sessionEvents = append(sessionEvents, &outbox.OutboxEvent{
					EventID:       uuid.New().String(),
					EventType:     string(event.TypeNotificationPush),
					Payload:       notifEventBody,
					AggregateType: "user",
					AggregateID:   fmt.Sprintf("%d", newSessionEv.UserId),
					CreatedAt:     time.Now(),
				})
			}

			if len(sessionEvents) > 0 {
				err = h.outboxSvc.InsertOutboxEvents(ctx, sessionEvents)
				if err != nil {
					h.logger.Error("failed to insert outbox events", "error", err)
					return err
				}
			}
		}

		return nil
	}
}

func (h *Handler) handlePushNotification(ctx context.Context, ev *commonpb.EventEnvelope) error {
	var notifEv notificationpb.NotificationEvent
	err := ev.Payload.UnmarshalTo(&notifEv)
	if err != nil {
		h.logger.Error("failed to unmarshal push notification event", "error", err)
		return err
	}

	switch payload := notifEv.Payload.(type) {
	case *notificationpb.NotificationEvent_SessionPayload:
		msg := fmt.Sprintf("user %d logged in with session %s", payload.SessionPayload.UserId, payload.SessionPayload.SessionId)
		if err := h.sender.Send(ctx, fmt.Sprintf("%d", payload.SessionPayload.UserId), "new notification", msg); err != nil {
			h.logger.Error("send notification failed", "error", err)
			return err
		}
		return nil
	default:
		return nil
	}
}
