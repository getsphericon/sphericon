package workspaceview_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/internal/domain/identity/memberships"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/testhelper"
	"github.com/getsphericon/sphericon/internal/usecase/workspaceview"
)

// updateAs saves the Workspace's name and postal address as the member userID does in the settings,
// on the revision the Workspace reads now.
func updateAs(t *testing.T, env *testhelper.TestEnv, userID int64, publicID, address string) (*ent.Workspace, error) {
	t.Helper()
	s, m, err := testhelper.Module[*memberships.Module](env).Scope(t.Context(), userID, publicID)
	require.NoError(t, err)
	ws := env.DB.Workspace.GetX(t.Context(), m.WorkspaceID)
	v, err := testhelper.Module[*workspaceview.UseCase](env).Update(t.Context(), s, workspaceview.Caller{Membership: m},
		workspaceview.UpdateInput{Revision: ws.Revision, Name: ws.Name, PostalAddress: &address})
	return v.Summary.Workspace, err
}

// A postal address is trimmed, so one of whitespace alone clears it, as an empty one does. Globex
// has no marketing email armed, so it may be cleared.
func TestUpdateTrimsThePostalAddress(t *testing.T) {
	env := testhelper.Setup(t)

	set, setErr := updateAs(t, env, fixtures.OwnerJaneID, fixtures.GlobexPublicID, "  1 Main St  ")
	cleared, clearErr := updateAs(t, env, fixtures.OwnerJaneID, fixtures.GlobexPublicID, "   ")

	require.NoError(t, setErr)
	require.NoError(t, clearErr)
	assert.Equal(t, "1 Main St", set.PostalAddress)
	assert.Empty(t, cleared.PostalAddress)
}

// Only taking away an address the Workspace has is checked: a Workspace with none, whose settings
// form sends the empty address with every rename, is renamed though its email is armed.
func TestARenameWithoutAPostalAddressIsNotRefusedWhileEmailIsArmed(t *testing.T) {
	env := testhelper.Setup(t)
	env.ClearPostalAddress(t, fixtures.AcmeID)

	renamed, err := updateAs(t, env, fixtures.OwnerJohnID, fixtures.AcmePublicID, "")

	require.NoError(t, err)
	assert.Empty(t, renamed.PostalAddress)
}
