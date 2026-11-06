// Package postgres holds the two server-level statements the rest of the code needs and ent cannot
// express: the transaction-scoped advisory lock and the server version. Each is written here once;
// callers pass any client that runs a statement (*sql.DB, *sql.Tx, *ent.Client, *ent.Tx).
package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

// Execer runs a statement that returns no rows.
type Execer interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
}

// Queryer runs a statement that returns rows.
type Queryer interface {
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
}

// LockTx takes the advisory lock key and holds it to the end of the surrounding transaction. Use it
// to make a value taken inside the transaction commit in the order it was taken.
func LockTx(ctx context.Context, e Execer, key int64) error {
	// raw sql: pg_advisory_xact_lock is a server function; ent has no advisory lock.
	_, err := e.ExecContext(ctx, "SELECT pg_advisory_xact_lock($1)", key)
	return err
}

// ServerMajor is the major version of the Postgres server (server_version_num is
// major*10000+minor from version 10 on).
func ServerMajor(ctx context.Context, q Queryer) (int, error) {
	// raw sql: SHOW is a server command; ent has no form for it.
	rows, err := q.QueryContext(ctx, "SHOW server_version_num")
	if err != nil {
		return 0, fmt.Errorf("read the Postgres version: %w", err)
	}
	defer func() { _ = rows.Close() }()
	if !rows.Next() {
		return 0, errors.Join(errors.New("read the Postgres version: no row"), rows.Err())
	}
	var num int
	if err := rows.Scan(&num); err != nil {
		return 0, fmt.Errorf("read the Postgres version: %w", err)
	}
	return num / 10000, rows.Err()
}
