package testhelper

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/internal/domain/identity/invitations"
	"github.com/getsphericon/sphericon/internal/domain/identity/joindomains"
	"github.com/getsphericon/sphericon/internal/domain/identity/memberships"
	"github.com/getsphericon/sphericon/internal/domain/identity/users"
	"github.com/getsphericon/sphericon/internal/infra/events"
	"github.com/getsphericon/sphericon/internal/infra/postgres"
)

// A race is run on a committed env (Setup with Committed): the Instance as the container builds it,
// over a database whose transactions really meet. The test holds a transaction open (a lock, or rows
// it wrote), queues the writers behind it and lets it go, so the writers meet in the order they
// queued, each already inside its transaction, on the bus, the hooks and the modules production has.

// Writer is one side of a race: a call that opens a transaction of its own, named by the test.
type Writer struct {
	Name string
	Run  func() error
}

// raceWait is how long a writer may take to start waiting, or a queued writer to finish once the
// hold is let go.
const raceWait = 10 * time.Second

// QueueBehind runs the writers in their order behind the transactions held open when it is called
// (the holders: the one a HoldOpen or the test's own code holds), each started once the ones before
// it wait, and then calls release: the writers meet in the order they queued, each already inside
// its transaction. A writer waits behind the holders when Postgres says so: a holder or a writer
// queued before it blocks it (pg_blocking_pids). A writer that finishes instead of waiting fails the
// test, since it took no lock the hold stands in; so does one that waits only for a transaction held
// open that is no holder, which the release never lets go. It returns each writer's error by its
// name, which is unique.
func (env *TestEnv) QueueBehind(tb testing.TB, release func(), writers ...Writer) map[string]error {
	tb.Helper()
	env.requireCommitted(tb, "QueueBehind")
	holders, err := idleInTransaction(context.WithoutCancel(tb.Context()), env.SQLDB)
	require.NoError(tb, err)
	return env.queueBehind(tb, holders, release, writers)
}

// QueueBehindWorkspaceLock runs the writers queued behind the lock of s's Workspace (ADR 0099),
// which it holds until all of them wait for it (QueueBehind): they meet in their order, and a
// writer that takes no Workspace lock fails the test. It returns each writer's error by its name.
func (env *TestEnv) QueueBehindWorkspaceLock(tb testing.TB, s *ent.Scoped, writers ...Writer) map[string]error {
	tb.Helper()
	env.requireCommitted(tb, "QueueBehindWorkspaceLock")
	holders, release := env.holding(tb, func() func() { return env.holdWorkspaceLock(tb, s) })
	return env.queueBehind(tb, holders, release, writers)
}

// QueueBehindKey runs the writers queued behind the advisory lock key, which it holds until all of
// them wait for it (QueueBehind): a writer that takes no lock of that key fails the test.
func (env *TestEnv) QueueBehindKey(tb testing.TB, key int64, writers ...Writer) map[string]error {
	tb.Helper()
	env.requireCommitted(tb, "QueueBehindKey")
	holders, release := env.holding(tb, func() func() { return env.holdKey(tb, key) })
	return env.queueBehind(tb, holders, release, writers)
}

// RunBesideKey runs run while it holds the advisory lock key in a transaction of its own, and
// returns run's error: run must finish without waiting for the key, so a writer that takes that lock
// fails the test (nothing arrives within the timeout). It is QueueBehindKey's other side: a write
// that must take no lock of the key.
func (env *TestEnv) RunBesideKey(tb testing.TB, key int64, run func() error) error {
	tb.Helper()
	env.requireCommitted(tb, "RunBesideKey")
	release := sync.OnceFunc(env.holdKey(tb, key))
	tb.Cleanup(release) // a writer stuck behind the key must not outlive the test
	defer release()
	result := make(chan error, 1)
	go func() { result <- run() }()
	return Receive(tb, result, "the writer finishes while the key is held")
}

// holding runs hold and returns the transaction it holds open, told apart from those held before
// it, and its release.
func (env *TestEnv) holding(tb testing.TB, hold func() (release func())) (holders []int, release func()) {
	tb.Helper()
	before, err := idleInTransaction(context.WithoutCancel(tb.Context()), env.SQLDB)
	require.NoError(tb, err)
	release = hold()
	after, err := idleInTransaction(context.WithoutCancel(tb.Context()), env.SQLDB)
	require.NoError(tb, err)
	for _, pid := range after {
		if !slices.Contains(before, pid) {
			holders = append(holders, pid)
		}
	}
	return holders, release
}

// queueBehind is QueueBehind behind the holders.
func (env *TestEnv) queueBehind(tb testing.TB, holders []int, release func(), writers []Writer) map[string]error {
	tb.Helper()
	release = sync.OnceFunc(release)
	tb.Cleanup(release) // a failed assertion must not leave the writers waiting
	require.NotEmpty(tb, holders, "no transaction is held open for the writers to queue behind")
	// The writers and the held transactions outlive the test body: a cleanup releases them after
	// tb.Context() is done, so every context of a race keeps its values and drops its cancel.
	ctx := context.WithoutCancel(tb.Context())
	queued := slices.Clone(holders)
	results := make(map[string]chan error, len(writers))
	for _, w := range writers {
		require.NotContains(tb, results, w.Name, "two writers are named %s", w.Name)
		result := make(chan error, 1)
		results[w.Name] = result
		go func() { result <- w.Run() }()
		var (
			waiter   int
			finished bool
			ended    error
			stray    error
		)
		require.Eventually(tb, func() bool {
			select {
			case ended = <-result:
				finished = true
				return true
			default:
			}
			var err error
			waiter, err = nextWaiter(ctx, env.SQLDB, queued)
			stray = err
			return err != nil || waiter != 0
		}, raceWait, 5*time.Millisecond, "%s waits behind the hold", w.Name)
		require.False(tb, finished, "%s ran at once instead of waiting for the hold (it returned %v)", w.Name, ended)
		require.NoError(tb, stray, "%s waits behind the hold", w.Name)
		queued = append(queued, waiter)
	}
	release()
	errs := make(map[string]error, len(results))
	for name, result := range results {
		errs[name] = Receive(tb, result, name+" finishes once the hold is let go")
	}
	return errs
}

// HoldOpen runs write in a transaction of the bus over the scope s and, once write returns, holds
// the transaction open and uncommitted until the returned release is called, which commits it. A
// concurrent writer that meets its rows waits for the release, as one that loses a race to a write
// in flight does. An error of write ends the test. The release runs at cleanup if the test does not
// call it.
func (env *TestEnv) HoldOpen(tb testing.TB, s *ent.Scoped, write func(ts *ent.Scoped, pub events.Publisher) error) (release func()) {
	tb.Helper()
	env.requireCommitted(tb, "HoldOpen")
	return holdTx(tb, "the held transaction's write", func(held func() error) error {
		return env.Bus.WithinScopedTx(context.WithoutCancel(tb.Context()), s, func(ts *ent.Scoped, pub events.Publisher) error {
			if err := write(ts, pub); err != nil {
				return err
			}
			return held()
		})
	})
}

// holdKey takes the advisory lock key in a transaction of its own and holds it until the returned
// release is called.
func (env *TestEnv) holdKey(tb testing.TB, key int64) (release func()) {
	tb.Helper()
	return holdTx(tb, "take the advisory lock", func(held func() error) error {
		tx, err := env.SQLDB.BeginTx(context.WithoutCancel(tb.Context()), nil)
		if err != nil {
			return err
		}
		if err := postgres.LockTx(context.WithoutCancel(tb.Context()), tx, key); err != nil {
			return fmt.Errorf("%w (rollback: %w)", err, tx.Rollback())
		}
		if err := held(); err != nil {
			return err
		}
		return tx.Commit()
	})
}

// holdWorkspaceLock takes the Workspace's lock (events.Bus.WithinWorkspaceLockTx) in a transaction
// of its own and holds it until the returned release is called, as a removal or a write that
// creates Live use holds it while it runs.
func (env *TestEnv) holdWorkspaceLock(tb testing.TB, s *ent.Scoped) (release func()) {
	tb.Helper()
	return holdTx(tb, "take the Workspace lock", func(held func() error) error {
		return env.Bus.WithinWorkspaceLockTx(context.WithoutCancel(tb.Context()), s, func(*ent.WorkspaceLocked, events.Publisher) error {
			return held()
		})
	})
}

// holdTx runs run on a goroutine of its own until run calls held, which blocks until the returned
// release; the release then waits for run to end and fails the test on its error, as does an error
// of run before it held (named by what). The release runs at cleanup if the test does not call it.
func holdTx(tb testing.TB, what string, run func(held func() error) error) (release func()) {
	tb.Helper()
	holding, let := make(chan struct{}), make(chan struct{})
	ended := make(chan error, 1)
	go func() {
		ended <- run(func() error {
			close(holding)
			<-let
			return nil
		})
	}()
	select {
	case <-holding:
	case err := <-ended:
		require.NoError(tb, err, what)
		tb.Fatalf("%s: ended without holding", what)
	}
	release = sync.OnceFunc(func() {
		close(let)
		require.NoError(tb, <-ended, what)
	})
	tb.Cleanup(release)
	return release
}

// requireCommitted fails the test at once on a rolled-back env, whose one connection would leave a
// race's writers waiting for each other for good.
func (env *TestEnv) requireCommitted(tb testing.TB, what string) {
	tb.Helper()
	if !env.committed {
		tb.Fatalf("testhelper: %s needs transactions that meet, on an env built with Setup(t, testhelper.Committed())", what)
	}
}

// idleInTransaction is the backends of db's database that hold a transaction open and run nothing:
// the transactions a race's writers can queue behind.
func idleInTransaction(ctx context.Context, db *sql.DB) ([]int, error) {
	// raw sql: pg_stat_activity is a server view; ent has no entity for it.
	return pids(ctx, db, "SELECT pid FROM pg_stat_activity WHERE datname = current_database() AND state = 'idle in transaction'")
}

// errStrayWait is a writer waiting for a transaction held open that is not in the queue: the
// release never lets that one go.
var errStrayWait = errors.New("it waits for a transaction held open that the hold does not stand in")

// nextWaiter is a backend of db's database, not yet in queued, that waits for a lock a backend of
// queued holds or waits for, or 0 when none does yet. It returns errStrayWait for a backend that
// waits only for transactions held open outside queued.
func nextWaiter(ctx context.Context, db *sql.DB, queued []int) (int, error) {
	// raw sql: pg_stat_activity and pg_blocking_pids are server views and functions; ent has no
	// entity for them.
	behind, err := pids(ctx, db, `SELECT pid FROM pg_stat_activity
		WHERE datname = current_database() AND wait_event_type = 'Lock'
		AND NOT pid = ANY($1::int[]) AND pg_blocking_pids(pid) && $1::int[]
		ORDER BY pid LIMIT 1`, queued)
	if err != nil || len(behind) > 0 {
		return first(behind), err
	}
	// raw sql: as above.
	stray, err := pids(ctx, db, `SELECT w.pid FROM pg_stat_activity w
		WHERE w.datname = current_database() AND w.wait_event_type = 'Lock'
		AND NOT w.pid = ANY($1::int[]) AND cardinality(pg_blocking_pids(w.pid)) > 0
		AND NOT pg_blocking_pids(w.pid) && $1::int[]
		AND NOT EXISTS (SELECT FROM pg_stat_activity b
			WHERE b.pid = ANY(pg_blocking_pids(w.pid)) AND b.state <> 'idle in transaction')
		LIMIT 1`, queued)
	if err != nil {
		return 0, err
	}
	if len(stray) > 0 {
		return 0, errStrayWait
	}
	return 0, nil
}

// pids runs query, which selects one column of backend ids.
func pids(ctx context.Context, db *sql.DB, query string, args ...any) ([]int, error) {
	// raw sql: runs the statement its caller wrote, with the reason written beside it.
	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var out []int
	for rows.Next() {
		var pid int
		if err := rows.Scan(&pid); err != nil {
			return nil, err
		}
		out = append(out, pid)
	}
	return out, rows.Err()
}

func first(xs []int) int {
	if len(xs) == 0 {
		return 0
	}
	return xs[0]
}

// ProvenUsers is the Users a Second factor or Passkeys module built by hand takes
// (secondfactors.Users, passkeys.Users): the env's users module, except that every identity proof
// holds, so a race test drives the factor's lifecycle, not the proof.
type ProvenUsers struct {
	*users.Module
}

// NewProvenUsers is ProvenUsers over env's users module, which publishes on env.Bus.
func NewProvenUsers(env *TestEnv) ProvenUsers {
	return ProvenUsers{Module: Module[*users.Module](env)}
}

// ProveIdentity holds for every User.
func (ProvenUsers) ProveIdentity(context.Context, *ent.User, users.IdentityProof) error { return nil }

// UsersModules are env's modules the users module writes Memberships and Invitations through, for a
// test that builds its own users module (users.Modules).
func UsersModules(env *TestEnv) users.Modules {
	return users.Modules{
		Memberships: Module[*memberships.Module](env),
		Invitations: Module[*invitations.Module](env),
		JoinDomains: Module[*joindomains.Module](env),
	}
}
