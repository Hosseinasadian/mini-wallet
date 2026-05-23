package http

import (
	"github.com/gin-gonic/gin"
	"net/http"
)

type Handler struct {
}

func NewHandler() Handler {
	return Handler{}
}

func (h *Handler) LiveHandler(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"status": "alive",
	})
}

func (h *Handler) ReadyHandler(c *gin.Context) {
	c.JSON(200, gin.H{
		"ready": true,
	})
}
