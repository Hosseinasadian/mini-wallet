package auth

const (
	ErrPasswordTooShort        = "password too short"
	ErrPasswordTooLong         = "password too long"
	ErrPasswordRequired        = "password is required"
	ErrInvalidEmail            = "invalid email"
	ErrEmailAlreadyExists      = "email already exists"
	ErrRegisterFailed          = "failed to register"
	ErrLoginFailed             = "failed to login"
	ErrInvalidLoginCredentials = "username or password is incorrect"
	ErrRefreshTokenFailed      = "failed to generate refresh token"
	ErrAccessTokenFailed       = "failed to generate access token"
	ErrGetSessionsFailed       = "failed to get sessions"
	ErrRevokeSessionFailed     = "failed to revoke session"
	ErrRevokeAllSessionsFailed = "failed to revoke all sessions"
)
