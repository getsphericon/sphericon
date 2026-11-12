package db

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"path"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/pressly/goose/v3"
)

// A migration waiting for a lock on a live table queues every query of the table behind it, so a
// migration that runs in a transaction waits at most lockTimeout. Postgres then rolls it back
// whole and the runner tries again after a pause of lockRetryFirst, doubling up to lockRetryMax,
// until its context ends: the queries the wait held are let through in between. A migration
// without a transaction (`-- +goose NO TRANSACTION`) keeps the wait unbounded: Postgres cannot
// roll it back, so a rerun would start a half-applied file over, and its `CONCURRENTLY`
// statements wait for older transactions by design, a wait that blocks no query.
const (
	lockTimeout    = 2 * time.Second
	lockRetryFirst = time.Second
	lockRetryMax   = 30 * time.Second
)

// lockNotAvailable is the SQLSTATE of a lock wait cut short by lock_timeout.
const lockNotAvailable = "55P03"

// setLockTimeout bounds the lock waits of the transaction it runs in.
var setLockTimeout = fmt.Sprintf("SET LOCAL lock_timeout = '%dms';", lockTimeout.Milliseconds())

// upWithLockRetries runs the pending migrations, starting over after a pause when a migration
// gives up waiting for a lock, and reports how many it applied in all.
func upWithLockRetries(ctx context.Context, provider *goose.Provider) (int, error) {
	applied := 0
	pause := lockRetryFirst
	for {
		results, err := provider.Up(ctx)
		applied += len(results)
		var partial *goose.PartialError
		if !errors.As(err, &partial) || !lockTimedOut(err) {
			return applied, err
		}
		applied += len(partial.Applied)
		name := path.Base(partial.Failed.Source.Path)
		slog.WarnContext(ctx, "a migration waited for a lock past its limit and is retried",
			"migration", name, "lock_timeout", lockTimeout, "retry_in", pause)
		select {
		case <-ctx.Done():
			return applied, fmt.Errorf("migration %s found its lock taken until %w: %w", name, ctx.Err(), err)
		case <-time.After(pause):
		}
		pause = min(2*pause, lockRetryMax)
	}
}

func lockTimedOut(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == lockNotAvailable
}

// lockBoundedFS is the migrations as goose reads them: a SQL file that runs in a transaction
// starts its Up section with setLockTimeout. Only the copy goose runs changes; the files, their
// lint and their checksums stay as written.
type lockBoundedFS struct{ fs.FS }

func (f lockBoundedFS) Open(name string) (fs.File, error) {
	if path.Ext(name) != ".sql" {
		return f.FS.Open(name)
	}
	src, err := fs.ReadFile(f.FS, name)
	if err != nil {
		return nil, err
	}
	info, err := fs.Stat(f.FS, name)
	if err != nil {
		return nil, err
	}
	bounded := boundLockWaits(src)
	return &memFile{Reader: bytes.NewReader(bounded), info: sizedInfo{FileInfo: info, size: int64(len(bounded))}}, nil
}

func (f lockBoundedFS) ReadDir(name string) ([]fs.DirEntry, error) { return fs.ReadDir(f.FS, name) }

// boundLockWaits puts setLockTimeout after the `-- +goose Up` marker of a migration that runs in
// a transaction, and leaves any other file as it is.
func boundLockWaits(src []byte) []byte {
	var out bytes.Buffer
	bounded := false
	for line := range strings.Lines(string(src)) {
		if strings.TrimSpace(line) == "-- +goose NO TRANSACTION" {
			return src
		}
		out.WriteString(line)
		if !bounded && strings.HasPrefix(strings.TrimSpace(line), "-- +goose Up") {
			if !strings.HasSuffix(line, "\n") {
				out.WriteString("\n")
			}
			out.WriteString(setLockTimeout + "\n")
			bounded = true
		}
	}
	return out.Bytes()
}

// lockBounded is a Go migration's transaction step with its lock waits bounded as a SQL
// migration's are.
func lockBounded(step func(context.Context, *sql.Tx) error) func(context.Context, *sql.Tx) error {
	return func(ctx context.Context, tx *sql.Tx) error {
		// raw sql: a session setting, which ent cannot express.
		if _, err := tx.ExecContext(ctx, setLockTimeout); err != nil {
			return err
		}
		return step(ctx, tx)
	}
}

type memFile struct {
	*bytes.Reader

	info fs.FileInfo
}

func (m *memFile) Stat() (fs.FileInfo, error) { return m.info, nil }
func (*memFile) Close() error                 { return nil }

type sizedInfo struct {
	fs.FileInfo

	size int64
}

func (s sizedInfo) Size() int64 { return s.size }
