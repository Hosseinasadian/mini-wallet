package repository

import (
	"context"
	"database/sql"
	"errors"
	"github.com/hosseinasadian/mini-wallet/pkg/richerror"
)

func (repo *Repository) RunProcess(ctx context.Context, messageId string, payload []byte) (err error) {
	const op richerror.Operation = "repository.RunProcess"

	tx, err := repo.db.BeginTxx(ctx, nil)
	if err != nil {
		return richerror.New(op).
			WithWrapper(err).
			WithKind(richerror.KindInternal).
			WithMessage("failed to start transaction")
	}

	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()

	var status string
	err = tx.QueryRowContext(ctx, `
		SELECT status FROM notification_processed_messages
		WHERE message_id = ?
	`, messageId).Scan(&status)

	if err != nil {
		if !errors.Is(err, sql.ErrNoRows) {
			return richerror.New(op).
				WithWrapper(err).
				WithKind(richerror.KindInternal).
				WithMessage("failed to check processed message")
		}

		_, err = tx.ExecContext(ctx, `
			INSERT IGNORE INTO notification_processed_messages 
				(message_id, status, event_payload, processing_started_at)
			VALUES (?, 'processing', ?, NOW())
		`, messageId, payload)
		if err != nil {
			return richerror.New(op).
				WithWrapper(err).
				WithKind(richerror.KindInternal).
				WithMessage("failed to insert message")
		}

	} else {
		if status == "sent" {
			return richerror.New(op).
				WithKind(richerror.KindConflict).
				WithMessage("message already processed")
		}

		_, err = tx.ExecContext(ctx, `
			UPDATE notification_processed_messages
			SET status = 'processing', event_payload = ?
			WHERE message_id = ?
		`, payload, messageId)
		if err != nil {
			return richerror.New(op).
				WithWrapper(err).
				WithKind(richerror.KindInternal).
				WithMessage("failed to reset message status")
		}
	}

	if err = tx.Commit(); err != nil {
		return richerror.New(op).
			WithWrapper(err).
			WithKind(richerror.KindInternal).
			WithMessage("failed to commit transaction")
	}

	return nil
}

func (repo *Repository) MarkSent(ctx context.Context, messageId string) error {
	const op = "repository.MarkSent"

	_, err := repo.db.ExecContext(ctx, `
		UPDATE notification_processed_messages
		SET status = 'completed', completed_at = NOW()
		WHERE message_id = ?
	`, messageId)
	if err != nil {
		return richerror.New(op).
			WithWrapper(err).
			WithKind(richerror.KindInternal).
			WithMessage("failed to mark message as sent")
	}

	return nil
}

func (repo *Repository) MarkFailed(ctx context.Context, messageId string) error {
	const op = "repository.MarkFailed"

	_, err := repo.db.ExecContext(ctx, `
		UPDATE notification_processed_messages
		SET status = 'failed'
		WHERE message_id = ?
	`, messageId)
	if err != nil {
		return richerror.New(op).
			WithWrapper(err).
			WithKind(richerror.KindInternal).
			WithMessage("failed to mark message as failed")
	}

	return nil
}
