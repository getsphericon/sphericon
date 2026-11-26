// Package postgres holds the server-level statements the rest of the code needs and ent cannot
// express: the transaction-scoped advisory lock and who holds an advisory lock, the session's
// backend pid and the server version. Each is written here once; callers pass any client that
// runs a statement (*sql.DB, *sql.Tx, *sql.Conn, *ent.Client, *ent.Tx).
package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"hash/fnv"
	"time"
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

// TryLockTx takes the advisory lock key, as LockTx does, only when no other transaction holds it,
// and reports whether it did: a caller that may come back later does not wait.
func TryLockTx(ctx context.Context, q Queryer, key int64) (bool, error) {
	// raw sql: pg_try_advisory_xact_lock is a server function; ent has no advisory lock.
	rows, err := q.QueryContext(ctx, "SELECT pg_try_advisory_xact_lock($1)", key)
	if err != nil {
		return false, err
	}
	defer func() { _ = rows.Close() }()
	var took bool
	if rows.Next() {
		err = rows.Scan(&took)
	}
	return took, errors.Join(err, rows.Err())
}

// LockHolder is a session that holds an advisory lock, as pg_stat_activity describes it.
type LockHolder struct {
	PID          int
	Application  string
	ClientAddr   string // empty over a Unix socket
	BackendStart time.Time
}

// LockHolders reads the sessions other than the one of pid exceptPID that hold the advisory lock
// key in the current database, by pid.
func LockHolders(ctx context.Context, q Queryer, key int64, exceptPID int) ([]LockHolder, error) {
	// raw sql: pg_locks and pg_stat_activity are server views; ent has no entity for them. An
	// advisory lock of a bigint key keeps its high half in classid, its low half in objid and 1 in
	// objsubid, and is scoped to a database: other databases of the server take the same key.
	rows, err := q.QueryContext(ctx, `
		SELECT l.pid, coalesce(a.application_name, ''), coalesce(host(a.client_addr), ''), a.backend_start
		FROM pg_locks l
		LEFT JOIN pg_stat_activity a ON a.pid = l.pid
		WHERE l.locktype = 'advisory' AND l.granted AND l.objsubid = 1 AND l.pid <> $2
			AND l.database = (SELECT oid FROM pg_database WHERE datname = current_database())
			AND ((l.classid::bigint << 32) | l.objid::bigint) = $1
		ORDER BY l.pid`, key, exceptPID)
	if err != nil {
		return nil, fmt.Errorf("read the advisory lock's holders: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var holders []LockHolder
	for rows.Next() {
		var h LockHolder
		var start sql.NullTime
		scanErr := rows.Scan(&h.PID, &h.Application, &h.ClientAddr, &start)
		if scanErr != nil {
			return nil, fmt.Errorf("read the advisory lock's holders: %w", scanErr)
		}
		h.BackendStart = start.Time
		holders = append(holders, h)
	}
	rowsErr := rows.Err()
	if rowsErr != nil {
		return nil, fmt.Errorf("read the advisory lock's holders: %w", rowsErr)
	}
	return holders, nil
}

// BackendPID is the pid of the server process of q's session: of one connection only when q is a
// *sql.Conn or a transaction.
func BackendPID(ctx context.Context, q Queryer) (int, error) {
	// raw sql: pg_backend_pid is a server function; ent has none.
	return queryInt(ctx, q, "SELECT pg_backend_pid()", "the backend pid")
}

// MinMajor is the oldest Postgres major the Instance runs on. The schema relies on 15 and later: a
// Workspace-owned table's composite foreign key names the one column its ON DELETE SET NULL clears
// (ADR 0017), and 16 is the oldest major still supported upstream for long enough to run on. `sphericon
// doctor` fails below it, and the self-hosting guide on the documentation site cites it.
const MinMajor = 16

// WorkspaceKey is the advisory lock key of one kind of lock of one Workspace: "workspace" for its
// removals and the writes that move Live use, "orders" for its order Events, "contacts-peak" for the
// Usage contacts peak. Every key LockTx takes for a Workspace comes from here, so the kinds hash
// apart, and two Workspaces whose keys collide only take turns.
func WorkspaceKey(kind string, workspaceID int64) int64 {
	h := fnv.New32a()
	_, _ = fmt.Fprintf(h, "%s:%d", kind, workspaceID)
	return int64(h.Sum32())
}

// DestinationKey is the advisory lock key of one kind of lock of one Destination of a Workspace on a
// Channel: "frequency-cap" for the count of its Frequency cap and the claims it is written with (ADR
// 0178). It is a 64-bit hash, apart from WorkspaceKey's, since a Workspace has many Destinations: two
// whose keys collide only wait for each other.
func DestinationKey(kind string, workspaceID int64, channel, destination string) int64 {
	h := fnv.New64a()
	_, _ = fmt.Fprintf(h, "%s:%d:%s:%s", kind, workspaceID, channel, destination)
	// The hash's 64 bits, as Postgres's bigint takes them.
	var key int64
	for _, b := range h.Sum(nil) {
		key = key<<8 | int64(b)
	}
	return key
}

// ServerMajor is the major version of the Postgres server (server_version_num is
// major*10000+minor from version 10 on).
func ServerMajor(ctx context.Context, q Queryer) (int, error) {
	// raw sql: SHOW is a server command; ent has no form for it.
	num, err := queryInt(ctx, q, "SHOW server_version_num", "the Postgres version")
	return num / 10000, err
}

// queryInt reads the one integer query returns, naming what in its errors. Queryer has only
// QueryContext, so the single row is read through it.
func queryInt(ctx context.Context, q Queryer, query, what string) (int, error) {
	// raw sql: runs the server statement its caller wrote, with the reason written beside it.
	rows, err := q.QueryContext(ctx, query)
	if err != nil {
		return 0, fmt.Errorf("read %s: %w", what, err)
	}
	defer func() { _ = rows.Close() }()
	if !rows.Next() {
		return 0, errors.Join(fmt.Errorf("read %s: no row", what), rows.Err())
	}
	var n int
	if err := rows.Scan(&n); err != nil {
		return 0, fmt.Errorf("read %s: %w", what, err)
	}
	return n, rows.Err()
}
