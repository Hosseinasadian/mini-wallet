package repository

import (
	"context"
	"fmt"
	"github.com/google/uuid"
	"github.com/hosseinasadian/mini-wallet/internal/auth/service/auth"
	"github.com/jmoiron/sqlx"
	"time"
)

type Repository struct {
	db *sqlx.DB
}

func NewRepository(db *sqlx.DB) *Repository {
	return &Repository{
		db: db,
	}
}

func (repo *Repository) CreateUserByEmailAndPassword(ctx context.Context, email, password string) (int64, error) {
	tx, err := repo.db.BeginTxx(ctx, nil)
	if err != nil {
		return 0, err
	}

	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()

	res, err := tx.ExecContext(ctx, "INSERT INTO users (email, password_hash) VALUES (?, ?)", email, password)
	if err != nil {
		return 0, err
	}

	id, err := res.LastInsertId()
	if err != nil {
		return 0, err
	}

	if err = tx.Commit(); err != nil {
		return 0, err
	}

	return id, nil
}

func (repo *Repository) GetUserByEmail(ctx context.Context, email string) (*auth.User, error) {
	var user auth.User
	err := repo.db.GetContext(ctx, &user, "SELECT * FROM users WHERE email=?", email)
	if err != nil {
		return nil, err
	}
	return &user, nil
}

func (repo *Repository) UpsertSession(ctx context.Context, deviceCtx *auth.DeviceContext, userID int64, refreshTokenHash string, expiresAt time.Time) (string, string, error) {

	tx, err := repo.db.BeginTxx(ctx, nil)
	if err != nil {
		return "", "", err
	}

	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback()
		}
	}()

	// ---------------------------------------------------
	// 1. UPSERT DEVICE
	// ---------------------------------------------------

	devicePublicID := uuid.NewString()

	_, err = tx.ExecContext(ctx, `
        INSERT INTO devices (
            public_id,
            installation_id,
            platform,
            device_name,
            app_version,
            last_seen_at
        )
        VALUES (?, ?, ?, ?, ?, NOW())
        ON DUPLICATE KEY UPDATE
            device_name = VALUES(device_name),
            app_version = VALUES(app_version),
            last_seen_at = NOW()
    `,
		devicePublicID,
		deviceCtx.InstallationID,
		deviceCtx.Platform,
		deviceCtx.DeviceName,
		deviceCtx.AppVersion,
	)
	if err != nil {
		return "", "", err
	}

	// fetch device (safe because UNIQUE(installation_id, platform))
	var deviceID int64
	err = tx.GetContext(ctx, &deviceID, `
        SELECT id
        FROM devices
        WHERE installation_id = ? AND platform = ?
    `,
		deviceCtx.InstallationID,
		deviceCtx.Platform,
	)
	if err != nil {
		return "", "", err
	}

	// ---------------------------------------------------
	// 2. CHECK EXISTING ACTIVE SESSION
	// ---------------------------------------------------

	var existingSession struct {
		PublicID string `db:"public_id"`
		UserID   int64  `db:"user_id"`
		DeviceID int64  `db:"device_id"`
	}

	err = tx.GetContext(ctx, &existingSession, `
        SELECT public_id, user_id, device_id
        FROM device_sessions
        WHERE user_id = ? AND device_id = ?
        LIMIT 1
    `,
		userID,
		deviceID,
	)

	// ---------------------------------------------------
	// 3. IF EXISTS → MOVE TO HISTORY + UPDATE
	// ---------------------------------------------------

	if err == nil {

		// move old session to history (before overwrite)
		_, _ = tx.ExecContext(ctx, `
            INSERT INTO device_session_history (
                session_public_id,
                user_id,
                device_id,
                refresh_token_hash,
                ip_address,
                user_agent,
                created_at,
                last_used_at,
                revoked_at,
                revoke_reason,
                revoked_by
            )
            SELECT
                public_id,
                user_id,
                device_id,
                refresh_token_hash,
                ip_address,
                user_agent,
                created_at,
                last_used_at,
                NOW(),
                'rotated',
                'system'
            FROM device_sessions
            WHERE public_id = ?
        `, existingSession.PublicID)

		// update active session
		_, err = tx.ExecContext(ctx, `
            UPDATE device_sessions
            SET refresh_token_hash = ?,
                expires_at = ?,
                ip_address = ?,
                user_agent = ?,
                last_used_at = NOW()
            WHERE public_id = ?
        `,
			refreshTokenHash,
			expiresAt,
			deviceCtx.IPAddress,
			deviceCtx.UserAgent,
			existingSession.PublicID,
		)
		if err != nil {
			return "", "", err
		}

		// commit
		if err = tx.Commit(); err != nil {
			return "", "", err
		}
		committed = true

		return devicePublicID, existingSession.PublicID, nil
	}

	// ---------------------------------------------------
	// 4. ELSE → CREATE NEW SESSION
	// ---------------------------------------------------

	sessionPublicID := uuid.NewString()

	_, err = tx.ExecContext(ctx, `
        INSERT INTO device_sessions (
            public_id,
            user_id,
            device_id,
            refresh_token_hash,
            ip_address,
            user_agent,
            expires_at,
            created_at,
            last_used_at
        )
        VALUES (?, ?, ?, ?, ?, ?, ?, NOW(), NOW())
    `,
		sessionPublicID,
		userID,
		deviceID,
		refreshTokenHash,
		deviceCtx.IPAddress,
		deviceCtx.UserAgent,
		expiresAt,
	)

	if err != nil {
		return "", "", err
	}

	// ---------------------------------------------------
	// COMMIT
	// ---------------------------------------------------

	if err = tx.Commit(); err != nil {
		return "", "", err
	}

	committed = true

	return devicePublicID, sessionPublicID, nil
}

func (repo *Repository) RotateRefreshToken(ctx context.Context, deviceCtx *auth.DeviceContext, oldRefreshTokenHash string, newRefreshTokenHash string, newExpiresAt time.Time) (string, string, int64, error) {

	tx, err := repo.db.BeginTxx(ctx, nil)
	if err != nil {
		return "", "", 0, err
	}

	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback()
		}
	}()

	// ---------------------------------------------------
	// 1. GET SESSION (NO expires check in SQL)
	// ---------------------------------------------------

	var sessionPublicID string
	var userID int64
	var deviceID int64
	var expiresAt time.Time

	err = tx.QueryRowxContext(ctx, `
        SELECT public_id, user_id, device_id, expires_at
        FROM device_sessions
        WHERE refresh_token_hash = ?
        FOR UPDATE
    `, oldRefreshTokenHash).
		Scan(&sessionPublicID, &userID, &deviceID, &expiresAt)

	// ---------------------------------------------------
	// 2. HANDLE NOT FOUND
	// ---------------------------------------------------

	if err != nil {
		return "", "", 0, fmt.Errorf("invalid refresh token")
	}

	// ---------------------------------------------------
	// 3. HANDLE EXPIRED TOKEN
	// ---------------------------------------------------

	if expiresAt.Before(time.Now()) {
		return "", "", 0, fmt.Errorf("refresh token expired")
	}

	// ---------------------------------------------------
	// 4. ROTATE TOKEN
	// ---------------------------------------------------

	_, err = tx.ExecContext(ctx, `
        UPDATE device_sessions
        SET refresh_token_hash = ?,
            expires_at = ?,
            ip_address = ?,
            user_agent = ?,
            last_used_at = NOW()
        WHERE public_id = ?
    `,
		newRefreshTokenHash,
		newExpiresAt,
		deviceCtx.IPAddress,
		deviceCtx.UserAgent,
		sessionPublicID,
	)

	if err != nil {
		return "", "", 0, err
	}

	// ---------------------------------------------------
	// 5. GET DEVICE PUBLIC ID (SEPARATE SIMPLE QUERY)
	// ---------------------------------------------------

	var devicePublicID string

	err = tx.GetContext(ctx, &devicePublicID, `
        SELECT public_id
        FROM devices
        WHERE id = ?
    `,
		deviceID,
	)

	if err != nil {
		return "", "", 0, err
	}

	// ---------------------------------------------------
	// 6. COMMIT
	// ---------------------------------------------------

	if err = tx.Commit(); err != nil {
		return "", "", 0, err
	}

	committed = true

	return devicePublicID, sessionPublicID, userID, nil
}

func (repo *Repository) GetUserSessions(ctx context.Context, userID int64) ([]auth.SessionItem, error) {

	var sessions []auth.SessionItem

	err := repo.db.SelectContext(ctx, &sessions, `
        SELECT
            ds.public_id AS session_id,
            d.public_id AS device_id,
            d.platform,
            d.device_name,
            d.app_version,
            ds.ip_address,
            ds.user_agent,
            ds.created_at,
            ds.last_used_at,
            ds.expires_at
        FROM device_sessions ds
        JOIN devices d ON d.id = ds.device_id
        WHERE ds.user_id = ?
        ORDER BY ds.last_used_at DESC
    `, userID)

	if err != nil {
		return nil, err
	}

	return sessions, nil
}

func (repo *Repository) RevokeSession(ctx context.Context, userID int64, sessionPublicID string, reason string, revokedBy string) error {

	tx, err := repo.db.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}

	committed := false

	defer func() {
		if !committed {
			_ = tx.Rollback()
		}
	}()

	// LOCK rows first (important)
	lockQuery := "SELECT id FROM device_sessions WHERE public_id = ? AND user_id = ? FOR UPDATE"

	if _, err = tx.ExecContext(ctx, lockQuery, sessionPublicID, userID); err != nil {
		return err
	}

	// move to history
	res, err := tx.ExecContext(ctx, `
        INSERT INTO device_session_history (
            session_public_id,
            user_id,
            device_id,
            refresh_token_hash,
            ip_address,
            user_agent,
            created_at,
            last_used_at,
            revoked_at,
            revoke_reason,
            revoked_by
        )
        SELECT
            public_id,
            user_id,
            device_id,
            refresh_token_hash,
            ip_address,
            user_agent,
            created_at,
            last_used_at,
            NOW(),
            ?,
            ?
        FROM device_sessions
        WHERE public_id = ?
          AND user_id = ?
    `,
		reason,
		revokedBy,
		sessionPublicID,
		userID,
	)

	if err != nil {
		return err
	}

	affected, err := res.RowsAffected()
	if err != nil {
		return err
	}

	if affected == 0 {
		return fmt.Errorf("session not found")
	}

	// delete active session

	_, err = tx.ExecContext(ctx, `
        DELETE FROM device_sessions
        WHERE public_id = ?
          AND user_id = ?
    `,
		sessionPublicID,
		userID,
	)

	if err != nil {
		return err
	}

	err = tx.Commit()
	if err != nil {
		return err
	}

	committed = true

	return nil
}

func (repo *Repository) RevokeAllSessions(ctx context.Context, userID int64, exceptSessionID *string, reason string, revokedBy string) error {

	tx, err := repo.db.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}

	committed := false

	defer func() {
		if !committed {
			_ = tx.Rollback()
		}
	}()

	whereClause := ""
	selectWhereClause := ""
	args := []interface{}{userID}
	selectArgs := []interface{}{reason, revokedBy, userID}

	if exceptSessionID != nil {
		whereClause = "AND public_id != ?"
		selectWhereClause = "AND public_id != ?"
		args = append(args, *exceptSessionID)
		selectArgs = append(args, *exceptSessionID)
	}

	// LOCK rows first (important)
	lockQuery := fmt.Sprintf(`
		SELECT id
		FROM device_sessions
		WHERE user_id = ?
		%s
		FOR UPDATE
	`, whereClause)

	if _, err = tx.ExecContext(ctx, lockQuery, selectArgs...); err != nil {
		return err
	}

	// archive
	query := fmt.Sprintf(`
        INSERT INTO device_session_history (
            session_public_id,
            user_id,
            device_id,
            refresh_token_hash,
            ip_address,
            user_agent,
            created_at,
            last_used_at,
            revoked_at,
            revoke_reason,
            revoked_by
        )
        SELECT
            public_id,
            user_id,
            device_id,
            refresh_token_hash,
            ip_address,
            user_agent,
            created_at,
            last_used_at,
            NOW(),
            ?,
            ?
        FROM device_sessions
        WHERE user_id = ?
        %s
    `, selectWhereClause)

	_, err = tx.ExecContext(ctx, query, selectArgs...)
	if err != nil {
		return err
	}

	// delete active sessions
	deleteQuery := fmt.Sprintf(`
        DELETE FROM device_sessions
        WHERE user_id = ?
        %s
    `, whereClause)

	_, err = tx.ExecContext(ctx, deleteQuery, args...)
	if err != nil {
		return err
	}

	err = tx.Commit()
	if err != nil {
		return err
	}

	committed = true

	return nil
}
