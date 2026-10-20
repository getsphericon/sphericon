package workspacedeletion

import (
	"context"
	"time"

	"github.com/qmuntal/stateless"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/internal/lifecycle"
)

// Storage and Machine expose the cycle's storage and table so a test can read the state of
// two movers before either writes.
func Storage(c *ent.Client, id int64, now time.Time) lifecycle.Storage[State] {
	return cycleStorage(c, id, now)
}

func Machine(s lifecycle.Storage[State]) *stateless.StateMachine { return cycleTable(s) }

// WithBatch returns a module that removes at most n rows per transaction, so a test exercises
// the batch loop on small fixtures.
func WithBatch(m *Module, n int) *Module {
	c := *m
	c.batch = n
	return &c
}

// StateOf is the cycle state a Workspace row is in.
func StateOf(ws *ent.Workspace) State { return stateOf(ws) }

// Tombstone is the erase move of one Workspace inside tx, after its rows are removed.
func Tombstone(ctx context.Context, tx *ent.Client, id int64, now time.Time) error {
	return tombstone(ctx, tx, id, now)
}
