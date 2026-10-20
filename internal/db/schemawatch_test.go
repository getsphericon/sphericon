package db_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/internal/db"
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
