package testhelper

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/internal/infra/events"
)

// UnderWorkspaceLock runs read in a transaction of the bus over s that holds the Workspace's lock
// (events.Bus.WithinWorkspaceLockTx, ADR 0108), as a removal runs the readers of what holds a row,
// and returns what read answered. The scope of the lock comes only from the bus, so a test of such
// a reader calls it through here.
func UnderWorkspaceLock[T any](t *testing.T, bus *events.Bus, s *ent.Scoped, read func(*ent.WorkspaceLocked) (T, error)) (T, error) {
	t.Helper()
	var (
		got    T
		answer error
	)
	err := bus.WithinWorkspaceLockTx(t.Context(), s, func(wl *ent.WorkspaceLocked, _ events.Publisher) error {
		got, answer = read(wl)
		return nil
	})
	require.NoError(t, err, "take the Workspace lock")
	return got, answer
}
