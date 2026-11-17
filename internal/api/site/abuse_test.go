package site_test

import (
	"testing"
	"time"

	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/config"
	siteapi "github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/domain/identity/suspension"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/testhelper"
	"github.com/getsphericon/sphericon/internal/usecase/needsattentionreport"
)

// The traffic the dashboard judges is in fixtures/events.yml, one scenario per day offset (the
// detector tests name them too): a test moves the clock back that many days, so the trailing
// hours hold only its own scenario. The volumes are small, so the tests judge with a floor of 20.
const warningDay = 30 // Vandelay: 3 of 40 bounced hard, in the warning tier

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

// The warning the report reads is the use case's (internal/usecase/needsattentionreport); the
// dashboard shows its rate as the report holds it.
func TestTheDashboardShowsTheWarningTierLive(t *testing.T) {
	env := testhelper.Setup(t, testhelper.WithClock(daysAgo(warningDay)), testhelper.WithStaffVolumeFloor(volumeFloor))
	held, ok := lo.Find(reportOf(t, env, fixtures.VandelayID), func(i needsattentionreport.Item) bool {
		return i.Kind == needsattentionreport.KindDeliverabilityWarning
	})
	require.True(t, ok)

	item := warning(t, env, fixtures.VandelayOwnerArtEmail, fixtures.VandelayPublicID)

	require.NotNil(t, item)
	assert.Equal(t, siteapi.SiteAttentionSeverityDegraded, item.Severity)
	abuse := item.Abuse.Value
	assert.Equal(t, held.Abuse.Domain, abuse.Domain)
	assert.Equal(t, siteapi.AbuseMetricBounce, abuse.Metric)
	assert.InDelta(t, held.Abuse.Rate, abuse.Rate, 1e-9)
}

func TestASuspendedWorkspaceCarriesTheSupportContactForItsBanner(t *testing.T) {
	env := testhelper.Setup(t, testhelper.WithConfig(func(c *config.Config) { c.SupportEmail = "support@sphericon.test" }))
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)

	active := workspaceOf(t, c, fixtures.AcmePublicID)
	_, err := suspension.NewSuspender(env.Bus, time.Now).Suspend(t.Context(), fixtures.AcmeID, suspension.SuspendInput{By: suspension.System, Reason: "complaint rate"})
	require.NoError(t, err)
	suspended := workspaceOf(t, c, fixtures.AcmePublicID)

	assert.False(t, active.SupportEmail.Set, "a Workspace that can send needs no appeal address")
	assert.Equal(t, "support@sphericon.test", suspended.SupportEmail.Value)
}
