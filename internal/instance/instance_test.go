package instance_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent/membership"
	"github.com/getsphericon/sphericon/internal/accounts"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/instance"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// The oldest live Workspace's Owners decide: the fixtures' oldest is Acme (John owns it), the next
// Globex (Jane owns it), and discarding Acme passes the decision on.
func TestAuthorizePassesToTheNextOldestWorkspaceWhenTheOldestIsDeleted(t *testing.T) {
	ctx := t.Context()
	env := testhelper.Setup(t)
	mod := instance.New(env.DB, instance.Environment{}, testHost, nil)
	refusal := accounts.Require(membership.RoleMember, accounts.PermInstanceManage)

	require.NoError(t, mod.Authorize(ctx, fixtures.OwnerJohnID))
	require.ErrorIs(t, mod.Authorize(ctx, fixtures.OwnerJaneID), refusal)

	require.NoError(t, env.DB.Workspace.UpdateOneID(fixtures.AcmeID).SetDiscardedAt(time.Now()).Exec(ctx))

	require.ErrorIs(t, mod.Authorize(ctx, fixtures.OwnerJohnID), refusal)
	require.NoError(t, mod.Authorize(ctx, fixtures.OwnerJaneID))
}
