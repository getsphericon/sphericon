package site_test

import (
	"testing"
	"time"

	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/config"
	siteapi "github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/suspension"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// The traffic the dashboard judges is in fixtures/events.yml, one scenario per day offset (the
// detector tests name them too): a test moves the clock back that many days, so the trailing
// hours hold only its own scenario. The volumes are small, so the tests judge with a floor of 20.
const (
	belowFloorDay = 6  // Vandelay: 19 sent, every one bounced hard, under the floor
	warningDay    = 30 // Vandelay: 3 of 40 bounced hard, in the warning tier
)

const volumeFloor = 20

func daysAgo(days int) func() time.Time {
	return func() time.Time { return time.Now().Add(-time.Duration(days) * 24 * time.Hour) }
}

// warning is the deliverability warning among what needs attention in the Workspace, nil when
// there is none.
func warning(t *testing.T, env *testhelper.TestEnv, email string, workspaceID siteapi.WorkspaceId) *siteapi.SiteAttentionItem {
	t.Helper()
	items := attentionOf(t, env.SiteActor(t, email), workspaceID)
	found, ok := lo.Find(items, func(i siteapi.SiteAttentionItem) bool {
		return i.Kind == siteapi.SiteAttentionKindDeliverabilityWarning
	})
	return lo.Ternary(ok, &found, nil)
}

func TestTheDashboardShowsTheWarningTierLive(t *testing.T) {
	env := testhelper.Setup(t, testhelper.WithClock(daysAgo(warningDay)), testhelper.WithStaffVolumeFloor(volumeFloor))

	item := warning(t, env, fixtures.VandelayOwnerArtEmail, fixtures.VandelayPublicID)

	require.NotNil(t, item)
	assert.Equal(t, siteapi.SiteAttentionSeverityDegraded, item.Severity)
	abuse := item.Abuse.Value
	assert.Equal(t, "mail.vandelay.test", abuse.Domain)
	assert.Equal(t, siteapi.SiteAbuseMetricBounce, abuse.Metric)
	assert.InDelta(t, 3.0/40.0, abuse.Rate, 1e-9)
}

func TestTheDashboardShowsNoWarningBelowTheFloor(t *testing.T) {
	env := testhelper.Setup(t, testhelper.WithClock(daysAgo(belowFloorDay)), testhelper.WithStaffVolumeFloor(volumeFloor))

	assert.Nil(t, warning(t, env, fixtures.VandelayOwnerArtEmail, fixtures.VandelayPublicID))
}

func TestTheDashboardShowsNoWarningWhenUnlicensed(t *testing.T) {
	env := testhelper.Setup(t, testhelper.WithoutLicense(), testhelper.WithClock(daysAgo(warningDay)), testhelper.WithStaffVolumeFloor(volumeFloor))

	assert.Nil(t, warning(t, env, fixtures.VandelayOwnerArtEmail, fixtures.VandelayPublicID))
}

func TestASuspendedWorkspaceCarriesTheSupportContactForItsBanner(t *testing.T) {
	env := testhelper.Setup(t, testhelper.WithConfig(func(c *config.Config) { c.SupportEmail = "support@sphericon.test" }))
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)

	active := workspaceOf(t, c, fixtures.AcmePublicID)
	_, err := suspension.SuspendWorkspace(t.Context(), env.Bus, fixtures.AcmeID, suspension.System, "complaint rate")
	require.NoError(t, err)
	suspended := workspaceOf(t, c, fixtures.AcmePublicID)

	assert.False(t, active.SupportEmail.Set, "a Workspace that can send needs no appeal address")
	assert.Equal(t, "support@sphericon.test", suspended.SupportEmail.Value)
}
