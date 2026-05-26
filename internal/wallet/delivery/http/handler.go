package http

import (
	"github.com/gin-gonic/gin"
	wallet "github.com/hosseinasadian/mini-wallet/internal/wallet/service"
	"github.com/hosseinasadian/mini-wallet/pkg/logger"
	"github.com/hosseinasadian/mini-wallet/pkg/richerror"
	"net/http"
)

type Handler struct {
	walletService *wallet.Service
	logger        *logger.Logger
}

func NewHandler(walletService *wallet.Service, logger *logger.Logger) Handler {
	return Handler{
		walletService: walletService,
		logger:        logger,
	}
}

func (h *Handler) LiveHandler(c *gin.Context) {
	c.Status(http.StatusOK)
}

func (h *Handler) ReadyHandler(c *gin.Context) {
	err := h.walletService.IsReady(c.Request.Context())
	if err != nil {
		errRes := richerror.ErrHTTP(err)
		c.JSON(errRes.Code, errRes)
		return
	}

	c.Status(http.StatusOK)
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
