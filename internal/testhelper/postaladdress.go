package testhelper

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// ClearPostalAddress empties the postal address of a fixture Workspace: every fixture Workspace
// has one, so its marketing email can be armed (ADR 0156), and a test of the gate clears it first.
// It writes the row as it stands, past the Workspace settings' refusal to clear an address while
// marketing email is armed, as a test that puts a row into a state at a chosen moment does.
func (env *TestEnv) ClearPostalAddress(tb testing.TB, workspaceID int64) {
	tb.Helper()
	err := env.DB.Workspace.UpdateOneID(workspaceID).SetPostalAddress("").Exec(tb.Context())
	require.NoError(tb, err)
}
