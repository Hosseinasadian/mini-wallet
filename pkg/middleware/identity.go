package middleware

import (
	"github.com/gin-gonic/gin"
	"net/http"
)

func IdentityMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {

		installationID := c.GetHeader("X-Installation-Id")
		if installationID == "" {
			c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"error": "missing installation id"})
			return
		}

		if len(installationID) < 10 {
			c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"error": "invalid installation id"})
			return
		}

		c.Set("installation_id", installationID)
		c.Next()
	}
}

func GetIdentity(c *gin.Context) string {
	installationId, ok := c.Get("installation_id")
	if !ok {
		return ""
	}

	return installationId.(string)
}
