package auth

type RegisterRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}
type RegisterResponse struct {
	Message      string  `json:"message"`
	AccessToken  *string `json:"user_access_token"`
	RefreshToken *string `json:"refresh_token"`
	DeviceID     string  `json:"device_id"`
	SessionID    string  `json:"session_id"`
}

type LoginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}
type LoginResponse struct {
	AccessToken  string `json:"user_access_token"`
	RefreshToken string `json:"refresh_token"`
	DeviceID     string `json:"device_id"`
	SessionID    string `json:"session_id"`
	User         *User  `json:"user"`
}
type RefreshTokenResponse struct {
	AccessToken  string `json:"user_access_token"`
	RefreshToken string `json:"refresh_token"`
	DeviceID     string `json:"device_id"`
	SessionID    string `json:"session_id"`
}

type DeviceContext struct {
	UserAgent      string `json:"user_agent"`
	IPAddress      []byte `json:"ip_address"`
	InstallationID string `json:"installation_id"`
	Platform       string `json:"platform"`
	DeviceName     string `json:"device_name"`
	AppVersion     string `json:"app_version"`
}
