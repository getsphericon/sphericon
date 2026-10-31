package external_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/config"
	externalapi "github.com/getsphericon/sphericon/gen/external"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// workspaceOf reads the Workspace a token of Acme acts in. Any token reads it: the Workspace
// a token acts in is no scope's secret, as whoami is not.
func workspaceOf(t *testing.T, env *testhelper.TestEnv) *externalapi.WorkspaceResource {
	t.Helper()
	res, err := env.ExternalScoped(t, "contacts:read").WorkspaceGet(context.Background())
	require.NoError(t, err)
	ws, ok := res.(*externalapi.WorkspaceResource)
	require.Truef(t, ok, "got %T", res)
	return ws
}

// An agent reads the Send cap the platform set on its Workspace (ADR 0067) and the Workspace's
// sends over the last 24 hours, to explain or plan around a slow send.
func TestExternalWorkspaceCarriesItsSendCapAndTheLast24HourSends(t *testing.T) {
	env := testhelper.Setup(t)
	env.DB.Workspace.UpdateOneID(fixtures.AcmeID).SetSendCap(5000).ExecX(context.Background())

	ws := workspaceOf(t, env)

	assert.Equal(t, externalapi.WorkspaceId(fixtures.AcmePublicID), ws.ID)
	assert.Equal(t, fixtures.AcmeName, ws.Name)
	want := externalapi.NewOptNilSendCap(externalapi.SendCap{
		PerDay: 5000,
		// Three sends inside the window: two through the default Integration and one through the
		// SES one; the older ones are outside it.
		SentLast24h: 3,
	})
	assert.Equal(t, want, ws.SendCap)
}

func TestExternalWorkspaceWithoutASendCapReadsNull(t *testing.T) {
	env := testhelper.Setup(t)

	ws := workspaceOf(t, env)

	assert.True(t, ws.SendCap.IsNull(), "got %+v", ws.SendCap)
}

// On an Instance without Send caps a stored value has no effect, so none is shown either.
func TestExternalWorkspaceOnAnInstanceWithoutSendCapsReadsNull(t *testing.T) {
	env := testhelper.Setup(t, testhelper.WithMode(config.ModeSelfHosted))
	env.DB.Workspace.UpdateOneID(fixtures.AcmeID).SetSendCap(5000).ExecX(context.Background())

	ws := workspaceOf(t, env)

	assert.True(t, ws.SendCap.IsNull(), "got %+v", ws.SendCap)
}
