package instance_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/instance"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// The oldest live Workspace's Owners decide: the fixtures' oldest is Acme (John owns it), the next
// Globex (Jane owns it), and discarding Acme passes the decision on.
func TestIsOwnerPassesToTheNextOldestWorkspaceWhenTheOldestIsDeleted(t *testing.T) {
	ctx := t.Context()
	env := testhelper.Setup(t)
	mod := instance.New(env.DB, instance.Environment{}, testHost, nil)

	first, err := mod.IsOwner(ctx, fixtures.OwnerJohnID)
	require.NoError(t, err)
	second, err := mod.IsOwner(ctx, fixtures.OwnerJaneID)
	require.NoError(t, err)
	assert.True(t, first)
	assert.False(t, second)

	require.NoError(t, env.DB.Workspace.UpdateOneID(fixtures.AcmeID).SetDiscardedAt(time.Now()).Exec(ctx))

	first, err = mod.IsOwner(ctx, fixtures.OwnerJohnID)
	require.NoError(t, err)
	second, err = mod.IsOwner(ctx, fixtures.OwnerJaneID)
	require.NoError(t, err)
	assert.False(t, first)
	assert.True(t, second)
}
