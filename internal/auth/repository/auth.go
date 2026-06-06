package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/go-sql-driver/mysql"
	"github.com/google/uuid"
	"github.com/hosseinasadian/mini-wallet/internal/auth/service/auth"
	outboxService "github.com/hosseinasadian/mini-wallet/internal/auth/service/outbox"
	pkgLogger "github.com/hosseinasadian/mini-wallet/pkg/logger"
	"github.com/hosseinasadian/mini-wallet/pkg/richerror"
	"github.com/jmoiron/sqlx"
)

type Repository struct {
	db     *sqlx.DB
	logger *pkgLogger.Logger
}

func NewRepository(db *sqlx.DB, logger *pkgLogger.Logger) *Repository {
	return &Repository{
		db:     db,
		logger: logger,
	}
}

func (repo *Repository) RunInTx(ctx context.Context, fn func(exec auth.Repository) error) error {
	var err error
	var tx *sqlx.Tx
	tx, err = repo.db.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}

	defer func(err error) {
		if err != nil {
			_ = tx.Rollback()
		}
	}(err)

	if err = fn(&txRepository{tx: tx}); err != nil {
		return err
	}

	return tx.Commit()

}

func (repo *Repository) Ping(ctx context.Context) error {
	return repo.db.PingContext(ctx)
}

func (repo *Repository) GetUserByEmail(ctx context.Context, email string) (*auth.User, error) {
	const op = "repository.GetUserByEmail"

	var user auth.User
	err := repo.db.GetContext(ctx, &user, "SELECT * FROM users WHERE email=?", email)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, richerror.New(op).
				WithWrapper(err).
				WithMessage("user not found").
				WithKind(richerror.KindNotFound)
		}

		return nil, richerror.New(op).
			WithWrapper(err).
			WithMessage("failed to get user").
			WithKind(richerror.KindInternal)
	}
	return &user, nil
}

func (repo *Repository) RotateRefreshToken(ctx context.Context, deviceCtx *auth.DeviceContext, oldRefreshTokenHash string, newRefreshTokenHash string, newExpiresAt time.Time, pushToken *string) (string, string, int64, error) {
	const op = "Repository.RotateRefreshToken"

	tx, err := repo.db.BeginTxx(ctx, nil)
	if err != nil {
		return "", "", 0, richerror.New(op).
			WithWrapper(err).
			WithMessage("failed to rotate refresh token").
			WithKind(richerror.KindInternal)

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
		if errors.Is(err, sql.ErrNoRows) {
			return "", "", 0, richerror.New(op).
				WithWrapper(err).
				WithMessage("invalid refresh token").
				WithKind(richerror.KindNotFound)
		}

		return "", "", 0, richerror.New(op).
			WithWrapper(err).
			WithMessage("invalid refresh token").
			WithKind(richerror.KindInternal)
	}

	// ---------------------------------------------------
	// 3. HANDLE EXPIRED TOKEN
	// ---------------------------------------------------

	if expiresAt.Before(time.Now()) {
		return "", "", 0, richerror.New(op).
			WithWrapper(err).
			WithMessage("failed to rotate refresh token").
			WithKind(richerror.KindInternal)
	}

	// ---------------------------------------------------
	// 4. ROTATE TOKEN
	// ---------------------------------------------------

	if pushToken == nil {
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
	} else {
		var newPushToken interface{} = nil
		if *pushToken != "" {
			newPushToken = *pushToken
		}

		_, err = tx.ExecContext(ctx, `
        UPDATE device_sessions
        SET refresh_token_hash = ?,
            expires_at = ?,
            ip_address = ?,
            user_agent = ?,
            push_token = ?,
            last_used_at = NOW()
        WHERE public_id = ?
    `,
			newRefreshTokenHash,
			newExpiresAt,
			deviceCtx.IPAddress,
			deviceCtx.UserAgent,
			newPushToken,
			sessionPublicID,
		)
	}

	if err != nil {
		return "", "", 0, richerror.New(op).
			WithWrapper(err).
			WithMessage("failed to rotate refresh token").
			WithKind(richerror.KindInternal)
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
		return "", "", 0, richerror.New(op).
			WithWrapper(err).
			WithMessage("failed to rotate refresh token").
			WithKind(richerror.KindInternal)
	}

	// ---------------------------------------------------
	// 6. COMMIT
	// ---------------------------------------------------

	if err = tx.Commit(); err != nil {
		return "", "", 0, richerror.New(op).
			WithWrapper(err).
			WithMessage("failed to rotate refresh token").
			WithKind(richerror.KindInternal)
	}

	committed = true

	return devicePublicID, sessionPublicID, userID, nil
}

func (repo *Repository) GetUserSessions(ctx context.Context, userID int64) ([]auth.SessionItem, error) {
	const op = "Repository.GetUserSessions"

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
		return nil, richerror.New(op).
			WithWrapper(err).
			WithMessage("failed to get user sessions").
			WithKind(richerror.KindInternal)
	}

	return sessions, nil
}

func (repo *Repository) RevokeSession(ctx context.Context, userID int64, sessionPublicID string, reason string, revokedBy string) error {
	const op = "Repository.RevokeSession"

	tx, err := repo.db.BeginTxx(ctx, nil)
	if err != nil {
		return richerror.New(op).
			WithWrapper(err).
			WithMessage("failed to revoke session").
			WithKind(richerror.KindInternal)
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
		return richerror.New(op).
			WithWrapper(err).
			WithMessage("failed to revoke session").
			WithKind(richerror.KindInternal)
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
		return richerror.New(op).
			WithWrapper(err).
			WithMessage("failed to revoke session").
			WithKind(richerror.KindInternal)
	}

	affected, err := res.RowsAffected()
	if err != nil {
		return richerror.New(op).
			WithWrapper(err).
			WithMessage("failed to revoke session").
			WithKind(richerror.KindInternal)
	}

	if affected == 0 {
		return richerror.New(op).
			WithWrapper(err).
			WithMessage("failed to revoke session").
			WithKind(richerror.KindInternal)
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
		return richerror.New(op).
			WithWrapper(err).
			WithMessage("failed to revoke session").
			WithKind(richerror.KindInternal)
	}

	err = tx.Commit()
	if err != nil {
		return richerror.New(op).
			WithWrapper(err).
			WithMessage("failed to revoke session").
			WithKind(richerror.KindInternal)
	}

	committed = true

	return nil
}

func (repo *Repository) RevokeAllSessions(ctx context.Context, userID int64, exceptSessionID *string, reason string, revokedBy string) error {
	const op = "Repository.RevokeAllSessions"

	tx, err := repo.db.BeginTxx(ctx, nil)
	if err != nil {
		return richerror.New(op).
			WithWrapper(err).
			WithMessage("failed to revoke all sessions").
			WithKind(richerror.KindInternal)
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
		return richerror.New(op).
			WithWrapper(err).
			WithMessage("failed to revoke all sessions").
			WithKind(richerror.KindInternal)
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
		return richerror.New(op).
			WithWrapper(err).
			WithMessage("failed to revoke all sessions").
			WithKind(richerror.KindInternal)
	}

	// delete active sessions
	deleteQuery := fmt.Sprintf(`
        DELETE FROM device_sessions
        WHERE user_id = ?
        %s
    `, whereClause)

	_, err = tx.ExecContext(ctx, deleteQuery, args...)
	if err != nil {
		return richerror.New(op).
			WithWrapper(err).
			WithMessage("failed to revoke all sessions").
			WithKind(richerror.KindInternal)
	}

	err = tx.Commit()
	if err != nil {
		return richerror.New(op).
			WithWrapper(err).
			WithMessage("failed to revoke all sessions").
			WithKind(richerror.KindInternal)
	}

	committed = true

	return nil
}

func (repo *Repository) UpdatePushToken(ctx context.Context, sessionID string, userID int64, pushToken string) error {
	const op = "repository.UpdatePushToken"

	nullableToken := sql.NullString{String: pushToken, Valid: pushToken != ""}

	result, err := repo.db.ExecContext(ctx, `
        UPDATE device_sessions
        SET push_token = ?
        WHERE public_id = ? AND user_id = ?
    `, nullableToken, sessionID, userID)

	if err != nil {
		return richerror.New(op).
			WithWrapper(err).
			WithMessage("failed to update push token").
			WithKind(richerror.KindInternal)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return richerror.New(op).
			WithWrapper(err).
			WithMessage("failed to get rows affected").
			WithKind(richerror.KindInternal)
	}

	if rowsAffected == 0 {
		return richerror.New(op).
			WithMessage("session not found or does not belong to user").
			WithKind(richerror.KindNotFound)
	}

	return nil
}

type txRepository struct {
	tx *sqlx.Tx
}

func (repo *txRepository) CreateUserByEmailAndPassword(ctx context.Context, email, password string) (int64, error) {
	const op = "repository.CreateUserByEmailAndPassword"

	res, err := repo.tx.ExecContext(ctx, "INSERT INTO users (email, password_hash) VALUES (?, ?)", email, password)
	if err != nil {
		var mysqlErr *mysql.MySQLError
		if errors.As(err, &mysqlErr) && mysqlErr.Number == 1062 {
			return 0, richerror.New(op).
				WithMessage("duplicate entry").
				WithKind(richerror.KindConflict)
		}

		return 0, richerror.New(op).
			WithMessage("failed to execute insert query").
			WithKind(richerror.KindInternal)
	}

	id, err := res.LastInsertId()
	if err != nil {
		return 0, richerror.New(op).
			WithMessage("failed to get last insert id").
			WithKind(richerror.KindInternal)
	}

	return id, nil
}

func (repo *txRepository) UpsertSession(ctx context.Context, deviceCtx *auth.DeviceContext, userID int64, refreshTokenHash string, expiresAt time.Time, pushToken *string) (string, string, error) {
	const op = "repository.UpsertSession"

	// ---------------------------------------------------
	// 1. UPSERT DEVICE
	// ---------------------------------------------------

	devicePublicID := uuid.NewString()

	_, err := repo.tx.ExecContext(ctx, `
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
		return "", "", richerror.New(op).
			WithWrapper(err).
			WithMessage("failed to upsert session").
			WithKind(richerror.KindInternal)
	}

	// fetch device (safe because UNIQUE(installation_id, platform))
	var deviceID int64
	err = repo.tx.GetContext(ctx, &deviceID, `
        SELECT id
        FROM devices
        WHERE installation_id = ? AND platform = ?
    `,
		deviceCtx.InstallationID,
		deviceCtx.Platform,
	)
	if err != nil {
		return "", "", richerror.New(op).
			WithWrapper(err).
			WithMessage("failed to upsert session").
			WithKind(richerror.KindInternal)
	}

	// ---------------------------------------------------
	// 2. CHECK EXISTING ACTIVE SESSION
	// ---------------------------------------------------

	var existingSession struct {
		PublicID string `db:"public_id"`
		UserID   int64  `db:"user_id"`
		DeviceID int64  `db:"device_id"`
	}

	err = repo.tx.GetContext(ctx, &existingSession, `
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
		_, _ = repo.tx.ExecContext(ctx, `
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
		if pushToken == nil {
			_, err = repo.tx.ExecContext(ctx, `
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
		} else {
			var newPushToken interface{} = nil
			if *pushToken != "" {
				newPushToken = *pushToken
			}

			_, err = repo.tx.ExecContext(ctx, `
            UPDATE device_sessions
            SET refresh_token_hash = ?,
                expires_at = ?,
                ip_address = ?,
                user_agent = ?,
                push_token = ?,
                last_used_at = NOW()
            WHERE public_id = ?
        `,
				refreshTokenHash,
				expiresAt,
				deviceCtx.IPAddress,
				deviceCtx.UserAgent,
				newPushToken,
				existingSession.PublicID,
			)
		}

		if err != nil {
			return "", "", richerror.New(op).
				WithWrapper(err).
				WithMessage("failed to upsert session").
				WithKind(richerror.KindInternal)
		}

		return devicePublicID, existingSession.PublicID, nil
	}

	// ---------------------------------------------------
	// 4. ELSE → CREATE NEW SESSION
	// ---------------------------------------------------

	var newPushToken interface{} = nil
	if pushToken != nil && *pushToken != "" {
		newPushToken = *pushToken
	}

	sessionPublicID := uuid.NewString()

	_, err = repo.tx.ExecContext(ctx, `
        INSERT INTO device_sessions (
            public_id,
            user_id,
            device_id,
            refresh_token_hash,
            ip_address,
            user_agent,
			push_token,
            expires_at,
            created_at,
            last_used_at
        )
        VALUES (?, ?, ?, ?, ?, ?, ?, ?, NOW(), NOW())
    `,
		sessionPublicID,
		userID,
		deviceID,
		refreshTokenHash,
		deviceCtx.IPAddress,
		deviceCtx.UserAgent,
		newPushToken,
		expiresAt,
	)

	if err != nil {
		return "", "", richerror.New(op).
			WithWrapper(err).
			WithMessage("failed to upsert session").
			WithKind(richerror.KindInternal)
	}

	return devicePublicID, sessionPublicID, nil
}

func (repo *txRepository) InsertOutboxEvent(ctx context.Context, event *outboxService.OutboxEvent) error {
	const op richerror.Operation = "repository.InsertOutboxEvent"

	query := `
        INSERT INTO outbox_events (
            event_id,
            event_type,
            payload,
            aggregate_type,
            aggregate_id,
            created_at
        ) VALUES (?, ?, ?, ?, ?, ?)
    `

	_, err := repo.tx.ExecContext(ctx, query,
		event.EventID,
		event.EventType,
		event.Payload,
		event.AggregateType,
		event.AggregateID,
		event.CreatedAt,
	)

	if err != nil {
		return richerror.New(op).
			WithWrapper(err).
			WithMessage("failed to insert outbox event").
			WithKind(richerror.KindInternal)
	}

	return nil
}
