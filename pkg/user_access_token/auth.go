package user_access_token

import (
	"fmt"
	"github.com/golang-jwt/jwt/v5"
	"os"
	"time"
)

type Claims struct {
	jwt.RegisteredClaims
	AccountID int64  `json:"account_id"`
	SessionID string `json:"session_id"`
}

func GenerateAccessToken(accountID int64, sessionID string, duration time.Duration) (string, error) {
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, Claims{
		jwt.RegisteredClaims{
			Issuer:    "mini-wallet",
			Subject:   "access",
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(duration)),
		},
		accountID, sessionID,
	})

	return token.SignedString([]byte(os.Getenv("JWT_SECRET")))
}

func VerifyAccessToken(tokenString, jwtSecret string) (*Claims, error) {
	token, err := jwt.ParseWithClaims(tokenString, &Claims{}, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method")
		}
		return []byte(jwtSecret), nil
	})

	if err != nil {
		return nil, err
	}

	tokenClaims, ok := token.Claims.(*Claims)
	if !ok || !token.Valid {
		return nil, fmt.Errorf("token is invalid")
	}

	return tokenClaims, nil
}
