package site_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/config"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/suspension"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// The Send cap the platform set (ADR 0067) is shown to the Workspace read-only, with its
// last-24-hour sends; null when none applies.

func capWorkspace(t *testing.T, env *testhelper.TestEnv, workspaceID int64, sendCap int) {
	t.Helper()
	_, err := suspension.SetSendCap(t.Context(), env.Bus, workspaceID, suspension.StaffMember("op-1"), &sendCap)
	require.NoError(t, err)
}

func TestSiteWorkspaceShowsItsSendCapAndItsLastDaySends(t *testing.T) {
	env := testhelper.Setup(t)
	capWorkspace(t, env, fixtures.AcmeID, 1000)

	got := workspaceOf(t, env.SiteActor(t, fixtures.OwnerJohnEmail), fixtures.AcmePublicID).SendCap

	assert.False(t, got.Null)
	assert.Equal(t, int32(1000), got.Value.PerDay)
	assert.Equal(t, int32(3), got.Value.SentLast24h, "every Integration's sends of the last 24 hours (fixtures), an older one not")
}

func TestSiteWorkspaceWithoutASendCapShowsNull(t *testing.T) {
	env := testhelper.Setup(t)

	assert.True(t, workspaceOf(t, env.SiteActor(t, fixtures.OwnerJohnEmail), fixtures.AcmePublicID).SendCap.Null)
}

func TestSiteSelfHostedWorkspaceShowsNoSendCap(t *testing.T) {
	env := testhelper.Setup(t, testhelper.WithMode(config.ModeSelfHosted))
	capWorkspace(t, env, fixtures.AcmeID, 1000)

	assert.True(t, workspaceOf(t, env.SiteActor(t, fixtures.OwnerJohnEmail), fixtures.AcmePublicID).SendCap.Null,
		"a stored Send cap has no effect without Send caps, so none is shown")
}
