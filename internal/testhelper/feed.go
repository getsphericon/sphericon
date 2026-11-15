package testhelper

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/internal/infra/events"
)

// Feed makes a test's own write of rows the hooks watch (setup that marks a Workspace deleted,
// changes a User's address, adds or deletes a Contact, or the write a test of a Hook makes) as
// production makes every such write: in a transaction of the bus, whose client runs the Hooks and,
// right before the commit, the work they register (ent.Client.BeforeCommit). On a rolled-back env
// the transaction is a savepoint of the test's. An error of write ends the test. A module under
// test never gets the client write is handed: it takes env.DB, or a scope of it.
func (env *TestEnv) Feed(tb testing.TB, write func(tx *ent.Client) error) {
	tb.Helper()
	require.NoError(tb, env.Bus.WithinTx(tb.Context(), func(tx *ent.Client, _ events.Publisher) error {
		return write(tx)
	}), "the test's write through the bus")
}
