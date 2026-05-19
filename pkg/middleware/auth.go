package middleware

import (
	"github.com/gin-gonic/gin"
	"github.com/hosseinasadian/mini-wallet/pkg/user_access_token"
	"net/http"
	"strings"
)

func AuthMiddleware(jwtSecret string) gin.HandlerFunc {
	return func(c *gin.Context) {
		authHeader := c.GetHeader("Authorization")
		if authHeader == "" {
			c.JSON(http.StatusUnauthorized, gin.H{
				"message": "unauthorized",
			})
			c.Abort()
			return
		}

		parts := strings.Split(authHeader, " ")
		if len(parts) != 2 || parts[0] != "Bearer" {
			c.JSON(http.StatusUnauthorized, gin.H{
				"message": "unauthorized",
			})
			c.Abort()
			return
		}

		tokenClaims, err := user_access_token.VerifyAccessToken(parts[1], jwtSecret)
		if err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{
				"message": "unauthorized",
			})
			c.Abort()
			return
		}

		c.Set("account_id", tokenClaims.AccountID)
		c.Set("session_id", tokenClaims.SessionID)
		c.Next()
	}
}

func GetUserId(c *gin.Context) int64 {
	accountId, ok := c.Get("account_id")
	if !ok {
		return 0
	}

	return accountId.(int64)
}

func GetSessionId(c *gin.Context) string {
	sessionId, ok := c.Get("session_id")
	if !ok {
		return ""
	}

	return sessionId.(string)
}
