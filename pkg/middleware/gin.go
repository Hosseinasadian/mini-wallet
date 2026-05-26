package middleware

import (
	"github.com/gin-gonic/gin"
	pkgLogger "github.com/hosseinasadian/mini-wallet/pkg/logger"
	"net/http"
	"time"
)

func GinSlogLogger(logger *pkgLogger.Logger) gin.HandlerFunc {
	skipPaths := map[string]bool{
		"/live":  true,
		"/ready": true,
	}

	return func(c *gin.Context) {
		start := time.Now()
		c.Next()

		if skipPaths[c.Request.URL.Path] {
			return
		}

		logger.Info("http request",
			"method", c.Request.Method,
			"path", c.Request.URL.Path,
			"status", c.Writer.Status(),
			"latency", time.Since(start).String(),
			"request_id", GetCorrelationID(c.Request.Context()),
		)
	}
}

func GinSlogRecovery(logger *pkgLogger.Logger) gin.HandlerFunc {
	return gin.RecoveryWithWriter(nil, func(c *gin.Context, err any) {
		logger.Error("panic recovered",
			"error", err,
			"path", c.Request.URL.Path,
		)
		c.AbortWithStatus(http.StatusInternalServerError)
	})
}
