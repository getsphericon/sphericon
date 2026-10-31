package site_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/config"
	siteapi "github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// acmeWorkspace is Acme as John, its Owner, reads it from the Workspace list.
func acmeWorkspace(t *testing.T, env *testhelper.TestEnv) siteapi.SiteWorkspaceResource {
	t.Helper()
	list, err := env.SiteActor(t, fixtures.OwnerJohnEmail).SiteWorkspacesList(context.Background())
	require.NoError(t, err)
	require.Len(t, list, 1)
	return list[0]
}

// The members see the Send cap the platform set on their Workspace (ADR 0067) with the
// Workspace's sends over the last 24 hours, through every Integration.
func TestSiteWorkspaceCarriesItsSendCapAndTheLast24HourSends(t *testing.T) {
	env := testhelper.Setup(t)
	env.DB.Workspace.UpdateOneID(fixtures.AcmeID).SetSendCap(5000).ExecX(context.Background())

	got := acmeWorkspace(t, env)

	want := siteapi.NewOptNilSiteSendCap(siteapi.SiteSendCap{
		PerDay: 5000,
		// Three sends inside the window: two through the default Integration and one through the
		// SES one; the older ones are outside it.
		SentLast24h: 3,
	})
	assert.Equal(t, want, got.SendCap)
}

// A Workspace without a Send cap reads null: nothing suggests a limit that does not exist.
func TestSiteWorkspaceWithoutASendCapReadsNull(t *testing.T) {
	env := testhelper.Setup(t)

	got := acmeWorkspace(t, env)

	assert.True(t, got.SendCap.IsNull(), "got %+v", got.SendCap)
}

// On an Instance without Send caps a stored value has no effect, so none is shown either.
func TestSiteWorkspaceOnAnInstanceWithoutSendCapsReadsNull(t *testing.T) {
	env := testhelper.Setup(t, testhelper.WithMode(config.ModeSelfHosted))
	env.DB.Workspace.UpdateOneID(fixtures.AcmeID).SetSendCap(5000).ExecX(context.Background())

	got := acmeWorkspace(t, env)

	assert.True(t, got.SendCap.IsNull(), "got %+v", got.SendCap)
}
