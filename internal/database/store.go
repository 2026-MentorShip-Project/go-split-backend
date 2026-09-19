package database

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// Store permits handlers to use the request transaction or the connection pool.
type Store interface {
	Begin(context.Context) (pgx.Tx, error)
	Exec(context.Context, string, ...any) (pgconn.CommandTag, error)
	Query(context.Context, string, ...any) (pgx.Rows, error)
	QueryRow(context.Context, string, ...any) pgx.Row
}
type transactionKey struct{}

func WithTransaction(ctx context.Context, tx pgx.Tx) context.Context {
	return context.WithValue(ctx, transactionKey{}, tx)
}

type ContextStore struct{ Base Store }

func (s ContextStore) current(ctx context.Context) Store {
	if tx, ok := ctx.Value(transactionKey{}).(pgx.Tx); ok {
		return tx
	}
	return s.Base
}
func (s ContextStore) Begin(ctx context.Context) (pgx.Tx, error) { return s.current(ctx).Begin(ctx) }
func (s ContextStore) Exec(ctx context.Context, q string, args ...any) (pgconn.CommandTag, error) {
	return s.current(ctx).Exec(ctx, q, args...)
}
func (s ContextStore) Query(ctx context.Context, q string, args ...any) (pgx.Rows, error) {
	return s.current(ctx).Query(ctx, q, args...)
}
func (s ContextStore) QueryRow(ctx context.Context, q string, args ...any) pgx.Row {
	return s.current(ctx).QueryRow(ctx, q, args...)
}
