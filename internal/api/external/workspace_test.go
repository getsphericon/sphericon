package external_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/config"
	externalapi "github.com/getsphericon/sphericon/gen/external"
	siteapi "github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/domain/identity/apitokens"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// workspaceOf reads the Workspace a token of Acme acts in. Any token reads it: the Workspace
// a token acts in is no scope's secret, as whoami is not.
func workspaceOf(t *testing.T, env *testhelper.TestEnv) *externalapi.WorkspaceResource {
	t.Helper()
	res, err := env.ExternalScoped(t, apitokens.ScopeContactsRead).WorkspaceGet(context.Background())
	require.NoError(t, err)
	ws := res
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

// An agent reads whether the Workspace requires confirmed opt-in (ADR 0013) and has no operation
// that changes it: only a person turns it on, in the console.
func TestExternalWorkspaceCarriesConfirmedOptIn(t *testing.T) {
	env := testhelper.Setup(t)
	before := workspaceOf(t, env)
	owner := env.SiteActor(t, fixtures.OwnerJohnEmail)
	_, err := owner.SiteWorkspacesUpdateConfirmationEmail(t.Context(), &siteapi.SiteUpdateConfirmationEmailInput{
		Revision: siteapi.Revision(env.WorkspaceRevision(t.Context(), t, string(fixtures.AcmePublicID))), FromEmail: "hello@mail.acme.com",
	}, siteapi.SiteWorkspacesUpdateConfirmationEmailParams{WorkspaceId: fixtures.AcmePublicID})
	require.NoError(t, err)
	_, err = owner.SiteWorkspacesSetConfirmedOptIn(t.Context(), &siteapi.SiteSetConfirmedOptInInput{
		Revision: siteapi.Revision(env.WorkspaceRevision(t.Context(), t, string(fixtures.AcmePublicID))), Enabled: true,
	}, siteapi.SiteWorkspacesSetConfirmedOptInParams{WorkspaceId: fixtures.AcmePublicID})
	require.NoError(t, err)
	after := workspaceOf(t, env)

	assert.False(t, before.RequireConfirmedOptIn)
	assert.True(t, after.RequireConfirmedOptIn)
}

// An agent reads the Workspace's sending rules (ADR 0134, ADR 0137) to explain a marketing message
// that waits or is skipped; only a person changes them, in the console.
func TestExternalWorkspaceCarriesItsSendingRules(t *testing.T) {
	env := testhelper.Setup(t)

	ws := workspaceOf(t, env)

	wantCap := externalapi.FrequencyCap{Channel: externalapi.FrequencyCapChannelSMS, Country: externalapi.NilCountryCode{Null: true},
		MaxMessages: externalapi.NewNilFrequencyCapMaxMessages(3), PeriodMinutes: externalapi.NewNilFrequencyCapPeriodMinutes(1440)}
	assert.Equal(t, []externalapi.FrequencyCap{wantCap}, ws.SendingRules.FrequencyCaps)
	require.Len(t, ws.SendingRules.SendingWindows, 1)
	window := ws.SendingRules.SendingWindows[0]
	assert.Equal(t, externalapi.SendingWindowChannelSMS, window.Channel)
	assert.True(t, window.Country.Null)
	assert.Len(t, window.Hours, 7)
	assert.Equal(t, externalapi.SendingWindowHours{Day: externalapi.WeekdayMonday, OpensAt: 0, ClosesAt: 1440}, window.Hours[0],
		"Acme's window in the fixtures is open all day")
}
