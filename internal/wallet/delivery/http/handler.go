package http

import (
	"github.com/gin-gonic/gin"
	wallet "github.com/hosseinasadian/mini-wallet/internal/wallet/service"
	"net/http"
)

type Handler struct {
	walletService *wallet.Service
}

func NewHandler(walletService *wallet.Service) Handler {
	return Handler{
		walletService: walletService,
	}
}

func (h *Handler) PingHandler(c *gin.Context) {
	c.JSON(200, gin.H{
		"message": "pong",
	})
}

func (h *Handler) TransferHandler(c *gin.Context) {
	var req wallet.TransferRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"message": "invalid request",
		})
		return
	}
	
	response, err, code := h.walletService.Transfer(c.Request.Context(), 0, req)
	if err != nil {
		c.JSON(code, gin.H{
			"message": err.Error(),
		})
	} else {
		c.JSON(code, response)
	}
}
