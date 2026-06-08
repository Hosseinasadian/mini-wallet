package repository

import (
	"context"
	outboxService "github.com/hosseinasadian/mini-wallet/internal/auth/service/outbox"
	"github.com/hosseinasadian/mini-wallet/pkg/richerror"
	"github.com/jmoiron/sqlx"
	"time"
)

func (repo *Repository) ClaimOutboxEvents(ctx context.Context, count int64, lockTimeout time.Duration) ([]outboxService.OutboxEvent, error) {
	const op = "repository.ClaimOutboxEvents"

	tx, err := repo.db.BeginTxx(ctx, nil)
	if err != nil {
		return nil, richerror.New(op).
			WithWrapper(err).
			WithMessage("failed to get outbox events list").
			WithKind(richerror.KindInternal)
	}
	defer func() {
		_ = tx.Rollback()
	}()

	var events []outboxService.OutboxEvent
	query := `
        SELECT id, event_id, event_type, payload, aggregate_type, aggregate_id, created_at
        FROM auth_outbox_events
        WHERE status = 'pending'
        ORDER BY created_at ASC
        LIMIT ?
        FOR UPDATE SKIP LOCKED
    `
	if err = tx.SelectContext(ctx, &events, query, count); err != nil {
		return nil, richerror.New(op).
			WithWrapper(err).
			WithMessage("failed to get outbox events list").
			WithKind(richerror.KindInternal)
	}

	if len(events) == 0 {
		return nil, nil
	}

	ids := make([]string, len(events))
	for i, e := range events {
		ids[i] = e.EventID
	}

	query, args, err := sqlx.In(`
		UPDATE auth_outbox_events
		SET status = 'processing',
		    locked_until = DATE_ADD(NOW(), INTERVAL ? SECOND)
		WHERE event_id IN (?)
	`, int(lockTimeout.Seconds()), ids)
	if err != nil {
		return nil, richerror.New(op).
			WithWrapper(err).
			WithMessage("failed to get outbox events list").
			WithKind(richerror.KindInternal)
	}

	if _, err = tx.ExecContext(ctx, query, args...); err != nil {
		return nil, richerror.New(op).
			WithWrapper(err).
			WithMessage("failed to get outbox events list").
			WithKind(richerror.KindInternal)
	}

	return events, tx.Commit()
}

func (repo *Repository) ClaimSingleEvent(ctx context.Context, eventId string, lockTimeout time.Duration) error {
	const op = "repository.ClaimSingleEvent"

	res, err := repo.db.ExecContext(ctx, `
        UPDATE auth_outbox_events
        SET status = 'processing',
            locked_until = DATE_ADD(NOW(), INTERVAL ? SECOND)
        WHERE event_id = ?
          AND status = 'pending'
    `, int(lockTimeout.Seconds()), eventId)
	if err != nil {
		return richerror.New(op).
			WithWrapper(err).
			WithMessage("failed to claim outbox event").
			WithKind(richerror.KindInternal)
	}

	affected, err := res.RowsAffected()
	if err != nil {
		return richerror.New(op).
			WithWrapper(err).
			WithMessage("failed to get rows affected").
			WithKind(richerror.KindInternal)
	}

	if affected == 0 {
		return richerror.New(op).
			WithMessage("event already claimed or not found").
			WithKind(richerror.KindConflict)
	}

	return nil
}

func (repo *Repository) MarkOutboxEventProcessed(ctx context.Context, eventId string) error {
	const op = "repository.MarkOutboxEventProcessed"

	now := time.Now()
	_, err := repo.db.ExecContext(ctx, `
        UPDATE auth_outbox_events
        SET status = 'processed',
            processed_at = ?,
            locked_until = NULL
        WHERE event_id = ?
    `, now, eventId)

	if err != nil {
		return richerror.New(op).
			WithWrapper(err).
			WithMessage("failed to mark outbox event as processed").
			WithKind(richerror.KindInternal)
	}

	return nil
}

func (repo *Repository) MarkOutboxEventFailed(ctx context.Context, eventId string) error {
	const op = "repository.MarkOutboxEventFailed"

	_, err := repo.db.ExecContext(ctx, `
        UPDATE auth_outbox_events
        SET status = 'pending',
            locked_until = NULL
        WHERE event_id = ?
    `, eventId)

	if err != nil {
		return richerror.New(op).
			WithWrapper(err).
			WithMessage("failed to mark outbox event as failed").
			WithKind(richerror.KindInternal)
	}

	return nil
}

func (repo *Repository) MarkOutboxEventsProcessed(ctx context.Context, eventIds []string) error {
	const op = "repository.MarkOutboxEventsProcessed"

	if len(eventIds) == 0 {
		return nil
	}

	now := time.Now()
	query, args, err := sqlx.In(`
		UPDATE auth_outbox_events
		SET status = 'processed',
		    processed_at = ?,
		    locked_until = NULL
		WHERE event_id IN (?)
	`, now, eventIds)
	if err != nil {
		return richerror.New(op).
			WithWrapper(err).
			WithMessage("failed to build query").
			WithKind(richerror.KindInternal)
	}

	if _, err = repo.db.ExecContext(ctx, query, args...); err != nil {
		return richerror.New(op).
			WithWrapper(err).
			WithMessage("failed to mark outbox events as processed").
			WithKind(richerror.KindInternal)
	}

	return nil
}

func (repo *Repository) MarkOutboxEventsFailed(ctx context.Context, eventIds []string) error {
	const op = "repository.MarkOutboxEventsFailed"

	if len(eventIds) == 0 {
		return nil
	}

	query, args, err := sqlx.In(`
		UPDATE auth_outbox_events
		SET status = 'pending',
		    locked_until = NULL
		WHERE event_id IN (?)
	`, eventIds)
	if err != nil {
		return richerror.New(op).
			WithWrapper(err).
			WithMessage("failed to build query").
			WithKind(richerror.KindInternal)
	}

	if _, err = repo.db.ExecContext(ctx, query, args...); err != nil {
		return richerror.New(op).
			WithWrapper(err).
			WithMessage("failed to mark outbox events as failed").
			WithKind(richerror.KindInternal)
	}

	return nil
}

func (repo *Repository) RecoverStaleOutboxEvents(ctx context.Context) (int64, error) {
	const op = "repository.RecoverStaleOutboxEvents"

	res, err := repo.db.ExecContext(ctx, `
        UPDATE auth_outbox_events
        SET status = 'pending',
            locked_until = NULL
        WHERE status = 'processing'
          AND locked_until < NOW()
    `)

	if err != nil {
		return 0, richerror.New(op).
			WithWrapper(err).
			WithMessage("failed to recover stale outbox events").
			WithKind(richerror.KindInternal)
	}

	affected, err := res.RowsAffected()
	if err != nil {
		return 0, richerror.New(op).
			WithWrapper(err).
			WithMessage("failed to get rows affected").
			WithKind(richerror.KindInternal)
	}

	return affected, nil
}
