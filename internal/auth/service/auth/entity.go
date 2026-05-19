package auth

import "time"

type User struct {
	ID           int64     `json:"id" db:"id"`
	Email        string    `json:"email"  db:"email"`
	PasswordHash string    `json:"-" db:"password_hash"`
	CreatedAt    time.Time `json:"created_at" db:"created_at"`
}
type RefreshToken struct {
	ID        int64     `json:"-" db:"id"`
	UserID    int64     `json:"user_id" db:"user_id"`
	Token     string    `json:"token" db:"token"`
	Revoked   bool      `json:"revoked" db:"revoked"`
	ExpiresAt time.Time `json:"expires_at" db:"expires_at"`
	CreatedAt time.Time `json:"-" db:"created_at"`
}

type SessionItem struct {
	SessionID  string    `db:"session_id" json:"session_id"`
	DeviceID   string    `db:"device_id" json:"device_id"`
	Platform   string    `db:"platform" json:"platform"`
	DeviceName string    `db:"device_name" json:"device_name"`
	AppVersion string    `db:"app_version" json:"app_version"`
	IPAddress  []byte    `db:"ip_address" json:"ip_address"`
	UserAgent  string    `db:"user_agent" json:"user_agent"`
	CreatedAt  time.Time `db:"created_at" json:"created_at"`
	LastUsedAt time.Time `db:"last_used_at" json:"last_used_at"`
	ExpiresAt  time.Time `db:"expires_at" json:"expires_at"`
}
