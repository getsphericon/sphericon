package site_test

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/config"
	"github.com/getsphericon/sphericon/ent"
	siteapi "github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/events"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/suspension"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// layTraffic puts a Sending domain's last-hour traffic into the Workspace.
func layTraffic(t *testing.T, env *testhelper.TestEnv, ws int64, domain string, sent, complaints int) {
	t.Helper()
	s := env.DB.Scoped(ws)
	at := time.Now().Add(-time.Hour)
	var creates []*ent.EventScopedCreate
	for i := range sent {
		creates = append(creates, s.Event().Create().SetEmail(fmt.Sprintf("s%d@example.com", i)).
			SetAction(events.NameEmailSent).SetProperties(map[string]any{"sendingDomain": domain}).SetOccurredAt(at))
	}
	for i := range complaints {
		creates = append(creates, s.Event().Create().SetEmail(fmt.Sprintf("c%d@example.com", i)).
			SetAction(events.NameEmailComplained).SetProperties(map[string]any{"sendingDomain": domain}).SetOccurredAt(at))
	}
	require.NoError(t, s.Event().CreateBulk(creates...).Exec(t.Context()))
}

func overview(t *testing.T, env *testhelper.TestEnv, email, slug string) *siteapi.SiteAnalyticsOverview {
	t.Helper()
	res, err := env.SiteActor(t, email).SiteAnalyticsOverview(t.Context(), siteapi.SiteAnalyticsOverviewParams{Slug: slug})
	require.NoError(t, err)
	ov, ok := res.(*siteapi.SiteAnalyticsOverview)
	require.Truef(t, ok, "got %T", res)
	return ov
}

func TestTheDashboardShowsTheWarningTierLive(t *testing.T) {
	env := testhelper.Setup(t)
	layTraffic(t, env, fixtures.VandelayID, "mail.vandelay.test", 2000, 3) // 0.15% complaints

	warning := overview(t, env, fixtures.VandelayOwnerArtEmail, fixtures.VandelaySlug).AbuseWarning

	require.True(t, warning.Set)
	assert.Equal(t, "mail.vandelay.test", warning.Value.Domain)
	assert.Equal(t, siteapi.SiteAbuseMetricComplaint, warning.Value.Metric)
	assert.InDelta(t, 0.0015, warning.Value.Rate, 1e-9)
}

func TestTheDashboardShowsNoWarningBelowTheFloorOrWhenUnlicensed(t *testing.T) {
	below := testhelper.Setup(t)
	layTraffic(t, below, fixtures.VandelayID, "mail.vandelay.test", 999, 99)
	assert.False(t, overview(t, below, fixtures.VandelayOwnerArtEmail, fixtures.VandelaySlug).AbuseWarning.Set)

	unlicensed := testhelper.Setup(t, testhelper.WithoutLicense())
	layTraffic(t, unlicensed, fixtures.VandelayID, "mail.vandelay.test", 2000, 3)
	assert.False(t, overview(t, unlicensed, fixtures.VandelayOwnerArtEmail, fixtures.VandelaySlug).AbuseWarning.Set)
}

func TestASuspendedWorkspaceCarriesTheSupportContactForItsBanner(t *testing.T) {
	env := testhelper.Setup(t, testhelper.WithConfig(func(c *config.Config) { c.SupportEmail = "support@sphericon.test" }))
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)

	active := workspaceOf(t, c, fixtures.AcmeSlug)
	_, err := suspension.SuspendWorkspace(t.Context(), env.Bus, fixtures.AcmeID, suspension.System, "complaint rate")
	require.NoError(t, err)
	suspended := workspaceOf(t, c, fixtures.AcmeSlug)

	assert.False(t, active.SupportEmail.Set, "a Workspace that can send needs no appeal address")
	assert.Equal(t, "support@sphericon.test", suspended.SupportEmail.Value)
}
