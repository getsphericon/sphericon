package db_test

import (
	"context"
	"errors"
	"testing"
	"testing/synctest"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/internal/infra/db"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

type scriptedSchema struct {
	now     time.Time
	pending int
	err     error
}

func (s *scriptedSchema) watch() *db.SchemaWatch {
	return db.WatchSchema(func(context.Context) (int, error) {
		return s.pending, s.err
	}, func() time.Time { return s.now })
}

func TestSchemaWatchSeesAMigrationRunAfterABehindVerdictWithinASecond(t *testing.T) {
	s := &scriptedSchema{now: time.Unix(1000, 0), pending: 2}
	w := s.watch()

	behind, err := w.Pending(t.Context())
	require.NoError(t, err)
	assert.Equal(t, 2, behind)

	s.pending = 0
	s.now = s.now.Add(1500 * time.Millisecond)
	current, err := w.Pending(t.Context())
	require.NoError(t, err)
	assert.Equal(t, 0, current)
}

func TestSchemaWatchKeepsACurrentVerdictForAWhileThenAsksAgain(t *testing.T) {
	s := &scriptedSchema{now: time.Unix(1000, 0)}
	w := s.watch()
	_, err := w.Pending(t.Context())
	require.NoError(t, err)

	// The database was reset under the running process: the verdict holds until it expires.
	s.pending = 3
	s.now = s.now.Add(5 * time.Second)
	held, err := w.Pending(t.Context())
	require.NoError(t, err)
	assert.Equal(t, 0, held)

	s.now = s.now.Add(6 * time.Second)
	behind, err := w.Pending(t.Context())
	require.NoError(t, err)
	assert.Equal(t, 3, behind)
}

// blockingSchema is a database check that is held until released, and fails with its
// context's error if that context ends first.
type blockingSchema struct {
	entered chan struct{}
	release chan struct{}
}

func newBlockingSchema() *blockingSchema {
	return &blockingSchema{entered: make(chan struct{}, 1), release: make(chan struct{})}
}

func (s *blockingSchema) check(ctx context.Context) (int, error) {
	s.entered <- struct{}{}
	select {
	case <-s.release:
		return 2, nil
	case <-ctx.Done():
		return 0, ctx.Err()
	}
}

type verdict struct {
	pending int
	err     error
}

func pendingInto(ctx context.Context, w *db.SchemaWatch, out chan<- verdict) {
	pending, err := w.Pending(ctx)
	out <- verdict{pending, err}
}

// The request that started the check may go away (a closed tab, its timeout) while others wait
// for the same answer: the check outlives it, and every caller gets its verdict.
func TestSchemaWatchKeepsASharedCheckRunningWhenTheCallerThatStartedItIsCancelled(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		s := newBlockingSchema()
		w := db.WatchSchema(s.check, time.Now)
		first, cancelFirst := context.WithCancel(t.Context())
		starter := make(chan verdict, 1)
		go pendingInto(first, w, starter)
		testhelper.Receive(t, s.entered, "the first caller starts the check")
		others := make(chan verdict, 1)
		go pendingInto(t.Context(), w, others)
		synctest.Wait()

		cancelFirst()
		close(s.release)

		started := testhelper.Receive(t, starter, "the caller that started the check gets a verdict")
		require.NoError(t, started.err)
		assert.Equal(t, 2, started.pending)
		waited := testhelper.Receive(t, others, "the waiting caller gets a verdict")
		require.NoError(t, waited.err)
		assert.Equal(t, 2, waited.pending)
	})
}

func TestSchemaWatchDoesNotCacheAnError(t *testing.T) {
	s := &scriptedSchema{now: time.Unix(1000, 0), err: errors.New("db down")}
	w := s.watch()
	_, err := w.Pending(t.Context())
	require.Error(t, err)

	s.err = nil
	s.pending = 1
	behind, err := w.Pending(t.Context())
	require.NoError(t, err)
	assert.Equal(t, 1, behind)
}
