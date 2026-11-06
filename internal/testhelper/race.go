package testhelper

import (
	"context"
	"database/sql"
	"sync"
	"testing"
	"time"

	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/internal/infra/events"
)

// Writer is one side of a race: a call that opens a transaction of its own, named by the test.
type Writer struct {
	Name string
	Run  func() error
}

// QueuedBehind runs the writers in their order while a lock the test holds keeps them out, each
// started once the ones before it wait for a lock, and then calls release: the writers meet in the
// order they queued, each already inside its transaction. A writer that finishes instead of waiting
// fails the test, since it took no lock the hold stands in. probe is a pool on the test's own
// database (RealDB) to count the waiters with. It returns each writer's error by its name.
func QueuedBehind(t *testing.T, probe *sql.DB, release func(), writers ...Writer) map[string]error {
	t.Helper()
	release = sync.OnceFunc(release)
	t.Cleanup(release) // a failed assertion must not leave the writers waiting
	ctx := context.Background()
	results := make(map[string]chan error, len(writers))
	for i, w := range writers {
		result, done := make(chan error, 1), make(chan struct{})
		results[w.Name] = result
		go func() {
			result <- w.Run()
			close(done)
		}()
		waits := false
		require.Eventually(t, func() bool {
			select {
			case <-done:
				return true
			default:
			}
			n, err := LockWaiters(ctx, probe)
			waits = err == nil && n > i
			return waits
		}, 10*time.Second, 5*time.Millisecond, "%s finished or waits for a lock", w.Name)
		require.True(t, waits, "%s waits for the lock instead of running at once", w.Name)
	}
	release()
	errs := make(map[string]error, len(results))
	for name, result := range results {
		errs[name] = Receive(t, result, name+" finishes once the lock is let go")
	}
	return errs
}

// HoldWorkspaceLock takes the Workspace's lock (events.Bus.WithinWorkspaceLockTx) in a transaction
// of its own and holds it until the returned release is called, as a removal or a write that
// creates Live use holds it while it runs.
func HoldWorkspaceLock(t *testing.T, bus *events.Bus, s *ent.Scoped) (release func()) {
	t.Helper()
	held, let := make(chan struct{}), make(chan struct{})
	ended := make(chan error, 1)
	go func() {
		ended <- bus.WithinWorkspaceLockTx(context.Background(), s, func(*ent.Scoped, events.Publisher) error {
			close(held)
			<-let
			return nil
		})
	}()
	select {
	case <-held:
	case err := <-ended:
		require.NoError(t, err, "take the Workspace lock")
	}
	release = sync.OnceFunc(func() {
		close(let)
		require.NoError(t, <-ended)
	})
	t.Cleanup(release)
	return release
}

// AcceptingQueue is a job queue for a bus over a RealDB that takes every job and keeps none: a race
// test asks what the transactions committed, not what they queued.
type AcceptingQueue struct{ events.JobQueue } // only the inserts are called

// InsertTx accepts the job and inserts nothing.
func (AcceptingQueue) InsertTx(context.Context, *sql.Tx, river.JobArgs, *river.InsertOpts) (*rivertype.JobInsertResult, error) {
	return &rivertype.JobInsertResult{}, nil
}

// InsertManyTx accepts the jobs and inserts nothing.
func (AcceptingQueue) InsertManyTx(_ context.Context, _ *sql.Tx, params []river.InsertManyParams) ([]*rivertype.JobInsertResult, error) {
	out := make([]*rivertype.JobInsertResult, len(params))
	for i := range out {
		out[i] = &rivertype.JobInsertResult{}
	}
	return out, nil
}
