package postgres

import (
	"context"
	"errors"

	"git.platform.alem.school/amibragim/wheres-my-pizza/internal/domain/workers"
	"git.platform.alem.school/amibragim/wheres-my-pizza/internal/ports"

	"github.com/jackc/pgx/v5"
)

// WorkersRepo implements persistence for kitchen workers using pgx and SQL.
type WorkersRepo struct{}

// NewWorkersRepo constructs a new WorkersRepo.
func NewWorkersRepo() ports.WorkerRepository {
	return &WorkersRepo{}
}

// RegisterOnline registers a worker by name and type as online.
// Semantics:
//   - If no row exists -> INSERT (online) -> ok=true.
//   - If a row exists AND status='online' -> ok=false (duplicate) — caller should terminate.
//   - If a row exists AND status='offline' -> UPDATE to online -> ok=true.
func (r *WorkersRepo) RegisterOnline(ctx context.Context, name, typ string) (bool, error) {
	tx, err := MustTxFromContext(ctx)
	if err != nil {
		return false, err
	}

	// insert or update the worker row
	var status string
	err = tx.QueryRow(ctx, `
		INSERT INTO workers (name, type, status, last_seen)
		VALUES ($1, $2, 'online', now())
		ON CONFLICT (name) DO UPDATE
		  SET type = EXCLUDED.type,
		      status = 'online',
		      last_seen = now()
		  WHERE workers.status <> 'online'
		RETURNING status
	`, name, typ).Scan(&status)

	switch {
	case errors.Is(err, pgx.ErrNoRows):
		// the ON CONFLICT branch was a no-op because the row was already 'online'
		return false, nil
	case err != nil:
		// some other error occurred
		return false, err
	default:
		// inserted new row OR updated an offline row to online
		return true, nil
	}
}

// MarkOffline sets the worker's status to 'offline'.
func (r *WorkersRepo) MarkOffline(ctx context.Context, name string) error {
	tx, err := MustTxFromContext(ctx)
	if err != nil {
		return err
	}

	_, err = tx.Exec(ctx, `
		UPDATE workers
		SET status = 'offline', last_seen = now()
		WHERE name = $1
	`, name)
	return err
}

// Heartbeat refreshes the worker's last_seen and keeps it 'online'.
func (r *WorkersRepo) Heartbeat(ctx context.Context, name string) error {
	tx, err := MustTxFromContext(ctx)
	if err != nil {
		return err
	}

	_, err = tx.Exec(ctx, `
		UPDATE workers
		SET status = 'online', last_seen = now()
		WHERE name = $1
	`, name)
	return err
}

// IncrementProcessed atomically increases the processed counter for a worker.
func (r *WorkersRepo) IncrementProcessed(ctx context.Context, name string) error {
	tx, err := MustTxFromContext(ctx)
	if err != nil {
		return err
	}

	_, err = tx.Exec(ctx, `
		UPDATE workers
		SET orders_processed = orders_processed + 1
		WHERE name = $1
	`, name)
	return err
}

// ListAll returns all workers with their current runtime status.
func (r *WorkersRepo) ListAll(ctx context.Context) ([]workers.Worker, error) {
	tx, err := MustTxFromContext(ctx)
	if err != nil {
		return nil, err
	}

	rows, err := tx.Query(ctx, `
		SELECT id, name, type, status, last_seen, orders_processed
		FROM workers
		ORDER BY name ASC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []workers.Worker
	for rows.Next() {
		var w workers.Worker
		var status string

		if err := rows.Scan(&w.ID, &w.Name, &w.Type, &status, &w.LastSeen, &w.OrdersProcessed); err != nil {
			return nil, err
		}
		w.Status = workers.WorkerStatus(status)
		out = append(out, w)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return out, nil
}
