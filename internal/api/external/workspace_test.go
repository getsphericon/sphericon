package external_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/config"
	externalapi "github.com/getsphericon/sphericon/gen/external"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/suspension"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// An agent reads the Workspace's Send cap (ADR 0067) with any token, to explain a slow send.

func workspaceOverAPI(t *testing.T, env *testhelper.TestEnv) *externalapi.WorkspaceResource {
	t.Helper()
	res, err := env.ExternalScoped(t, "contacts:read").WorkspaceGet(t.Context())
	require.NoError(t, err)
	ws, ok := res.(*externalapi.WorkspaceResource)
	require.Truef(t, ok, "got %T", res)
	return ws
}

func capped(t *testing.T, env *testhelper.TestEnv, sendCap int) {
	t.Helper()
	_, err := suspension.SetSendCap(t.Context(), env.Bus, fixtures.AcmeID, suspension.StaffMember("op-1"), &sendCap)
	require.NoError(t, err)
}

func TestExternalWorkspaceGetShowsTheSendCap(t *testing.T) {
	env := testhelper.Setup(t)
	capped(t, env, 1000)

	ws := workspaceOverAPI(t, env)

	assert.Equal(t, externalapi.WorkspaceId(fixtures.AcmePublicID), ws.ID, "the token's Workspace")
	require.False(t, ws.SendCap.Null)
	assert.Equal(t, int32(1000), ws.SendCap.Value.PerDay)
	assert.Equal(t, int32(3), ws.SendCap.Value.SentLast24h)
}

func TestExternalWorkspaceGetWithoutASendCapIsNull(t *testing.T) {
	env := testhelper.Setup(t)

	assert.True(t, workspaceOverAPI(t, env).SendCap.Null)
}

func TestExternalSelfHostedWorkspaceHasNoSendCap(t *testing.T) {
	env := testhelper.Setup(t, testhelper.WithMode(config.ModeSelfHosted))
	capped(t, env, 1000)

	assert.True(t, workspaceOverAPI(t, env).SendCap.Null)
}
