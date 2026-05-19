package middleware

import (
	"github.com/gin-gonic/gin"
	"net/http"
)

func AppVersionMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {

		appVersion := c.GetHeader("X-App-Version")
		if appVersion == "" {
			c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"error": "missing app version"})
			return
		}

		c.Set("app_version", appVersion)
		c.Next()
	}
}

func GetAppVersion(c *gin.Context) string {
	appVersion, ok := c.Get("app_version")
	if !ok {
		return ""
	}

	return appVersion.(string)
}
