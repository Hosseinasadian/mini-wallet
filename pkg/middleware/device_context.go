package middleware

import (
	"github.com/gin-gonic/gin"
	"net"
)

func DeviceContextMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {

		userAgent := c.GetHeader("User-Agent")
		ipStr := c.ClientIP()

		ip := net.ParseIP(ipStr)
		var ipBytes []byte
		if ip != nil {
			ipBytes = ip.To16()
		}

		c.Set("user_agent", userAgent)
		c.Set("ip_address", ipBytes)

		c.Next()
	}
}

func GetUserAgent(c *gin.Context) string {
	userAgent, ok := c.Get("user_agent")
	if !ok {
		return ""
	}

	return userAgent.(string)
}

func GetIPAddress(c *gin.Context) []byte {
	ipAddress, ok := c.Get("ip_address")
	if !ok {
		return nil
	}

	ip, ok := ipAddress.([]byte)
	if !ok {
		return nil
	}

	return ip
}
