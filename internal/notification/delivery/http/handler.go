package http

import (
	"errors"
	"fmt"
	"github.com/gin-gonic/gin"
	"github.com/hosseinasadian/mini-wallet/internal/notification/service/notification"
	"github.com/hosseinasadian/mini-wallet/pkg/hub"
	"github.com/hosseinasadian/mini-wallet/pkg/middleware"
	"io"
	"net/http"
)

type Handler struct {
	notificationService *notification.Service
	notificationHub     *hub.Hub
}

func NewHandler(notificationService *notification.Service, notificationHub *hub.Hub) Handler {
	return Handler{notificationService: notificationService, notificationHub: notificationHub}
}

func (h *Handler) PingHandler(c *gin.Context) {
	c.JSON(200, gin.H{
		"message": "pong",
	})
}

// @Summary      Get Ticket
// @Description  Generate a short-lived ticket for SSE stream authentication
// @Tags         notification
// @Security     BearerAuth
// @Produce      json
// @Success      200  {object}  map[string]string
// @Failure      500  {object}  map[string]string
// @Router       /ticket [post]
func (h *Handler) TicketHandler(c *gin.Context) {
	ctx := c.Request.Context()
	accountId := middleware.GetUserId(c)

	ticket, err := h.notificationService.CreateTicket(ctx, accountId)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": err.Error(),
		})
		return
	}

	c.JSON(200, gin.H{
		"ticket": ticket,
	})
}

// @Summary      SSE Stream
// @Description  Connect to notification stream using a valid ticket.
// @Description  ⚠️ This is a Server-Sent Events endpoint. Test with EventSource or curl, not Swagger UI.
// @Description  Example: curl -N "http://localhost:180/notification/stream?ticket=YOUR_TICKET"
// @Tags         notification
// @Produce      text/event-stream
// @Param        ticket  query     string  true  "Short-lived ticket obtained from /ticket"
// @Success      200     {string}  string  "data: {event payload}"
// @Failure      400     {object}  map[string]string  "ticket required"
// @Failure      401     {object}  map[string]string  "invalid or expired ticket"
// @Failure      500     {object}  map[string]string
// @Router       /stream [get]
func (h *Handler) NotificationsHandler(c *gin.Context) {
	ctx := c.Request.Context()

	ticket := c.Query("ticket")
	if ticket == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "ticket required"})
		return
	}

	userId, err := h.notificationService.ConsumeTicket(ctx, ticket)
	if errors.Is(err, notification.ErrTicketNotFound) {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid or expired ticket"})
		return
	}

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("X-Accel-Buffering", "no")

	userIdStr := fmt.Sprintf("%d", userId)
	ch := h.notificationHub.Connect(userIdStr)
	defer h.notificationHub.Disconnect(userIdStr)

	c.Stream(func(w io.Writer) bool {
		select {
		case msg, ok := <-ch:
			if !ok {
				return false
			}
			c.SSEvent("notification", msg)
			return true
		case <-ctx.Done():
			return false
		}
	})
}
