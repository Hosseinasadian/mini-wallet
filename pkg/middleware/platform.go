package middleware

import (
	"github.com/gin-gonic/gin"
	"net/http"
)

func PlatformMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {

		platform := c.GetHeader("X-Platform")
		if platform == "" {
			c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"error": "missing platform id"})
			return
		}

		c.Set("platform", platform)
		c.Next()
	}
}

func GetPlatform(c *gin.Context) string {
	platform, ok := c.Get("platform")
	if !ok {
		return ""
	}

	return platform.(string)
}
