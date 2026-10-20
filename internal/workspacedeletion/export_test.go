package workspacedeletion

import (
	"time"

	"github.com/qmuntal/stateless"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/internal/lifecycle"
)

// Storage and Machine expose the cycle's storage and table so a test can read the state of
// two movers before either writes.
func Storage(c *ent.Client, id int64, now time.Time) lifecycle.Storage[State] {
	return storage(c, id, now)
}

func Machine(s lifecycle.Storage[State]) *stateless.StateMachine { return machine(s) }
