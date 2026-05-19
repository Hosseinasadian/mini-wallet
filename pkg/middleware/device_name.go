package middleware

import (
	"github.com/gin-gonic/gin"
	"net/http"
)

func DeviceNameMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {

		deviceName := c.GetHeader("X-Device-Name")
		if deviceName == "" {
			c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"error": "missing device name"})
			return
		}

		c.Set("device_name", deviceName)
		c.Next()
	}
}

func GetDeviceName(c *gin.Context) string {
	deviceName, ok := c.Get("device_name")
	if !ok {
		return ""
	}

	return deviceName.(string)
}
