package http

import (
	"github.com/gin-gonic/gin"
	"github.com/hosseinasadian/mini-wallet/pkg/logger"
	"net/http"
)

type Handler struct {
	logger *logger.Logger
}

func NewHandler(logger *logger.Logger) Handler {
	return Handler{
		logger: logger,
	}
}

func (h *Handler) LiveHandler(c *gin.Context) {
	c.Status(http.StatusOK)
}

func (h *Handler) ReadyHandler(c *gin.Context) {
	c.Status(http.StatusOK)
}
