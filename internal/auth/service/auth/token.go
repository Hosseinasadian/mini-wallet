package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"github.com/hosseinasadian/mini-wallet/pkg/richerror"
)

func generateRefreshToken() (string, error) {
	const op richerror.Operation = "auth.generateRefreshToken"

	bytes := make([]byte, 32)
	_, err := rand.Read(bytes)
	if err != nil {
		return "", richerror.New(op).
			WithWrapper(err).
			WithMessage(ErrRefreshTokenFailed).
			WithKind(richerror.KindInternal)
	}

	token := base64.URLEncoding.EncodeToString(bytes)
	return token, nil
}

func hashRefreshToken(token string) string {
	h := sha256.Sum256([]byte(token))
	return hex.EncodeToString(h[:])
}
