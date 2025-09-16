package postgres

import (
	"context"
	"errors"

	"git.platform.alem.school/amibragim/wheres-my-pizza/internal/ports"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ctxKey is an unexported key type for storing pgx.Tx in context.
// Using a distinct type avoids collisions with other context values.
type ctxKey struct{}

var txKey = ctxKey{}

// UnitOfWork coordinates transactional execution against a pgx pool.
type UnitOfWork struct {
	pool *pgxpool.Pool
}

// NewUnitOfWork constructs a UnitOfWork bound to the given pool.
func NewUnitOfWork(pool *pgxpool.Pool) ports.UnitOfWork {
	return &UnitOfWork{pool: pool}
}

// WithinTx executes fn within a database transaction.
//   - If a transaction already exists in ctx, fn is executed within that tx (nested calls are supported).
//   - If fn returns an error, the transaction is rolled back and the error is returned.
//   - If fn panics, the transaction is rolled back and the panic is rethrown.
//   - On success, the transaction is committed.
func (u *UnitOfWork) WithinTx(ctx context.Context, fn func(ctx context.Context) error) error {
	// if there is already a tx in the context, just run within it (support nesting).
	if _, ok := TxFromContext(ctx); ok {
		return fn(ctx)
	}

	// start a new transaction
	tx, err := u.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return err
	}

	// ensure rollback on panic, then rethrow panic.
	defer func() {
		if p := recover(); p != nil {
			_ = tx.Rollback(ctx)
			panic(p)
		}
	}()

	// run user function with tx injected into context.
	txCtx := context.WithValue(ctx, txKey, tx)
	if err := fn(txCtx); err != nil {
		_ = tx.Rollback(ctx)
		return err
	}

	return tx.Commit(ctx)
}

// TxFromContext extracts the current pgx.Tx from ctx if present.
// Returns (tx, true) when inside WithinTx, otherwise (nil, false).
func TxFromContext(ctx context.Context) (pgx.Tx, bool) {
	tx, ok := ctx.Value(txKey).(pgx.Tx)
	return tx, ok
}

// MustTxFromContext returns the active pgx.Tx or an error if none is found.
// Useful inside repository methods that must be called within a UnitOfWork.
func MustTxFromContext(ctx context.Context) (pgx.Tx, error) {
	if tx, ok := TxFromContext(ctx); ok {
		return tx, nil
	}
	return nil, errors.New("no transaction in context: call this repository within UnitOfWork.WithinTx")
}
