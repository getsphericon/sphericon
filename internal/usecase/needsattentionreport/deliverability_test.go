package needsattentionreport_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/internal/domain/insights/abusewarning"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/testhelper"
	"github.com/getsphericon/sphericon/internal/usecase/needsattentionreport"
)

// The traffic the warning tier judges is in fixtures/events.yml, one scenario per day offset (the
// detector tests name them too): a test moves the clock back that many days, so the trailing hours
// hold only its own scenario. The volumes are small, so the tests judge with a floor of 20.
const (
	belowFloorDay = 6  // Vandelay: 19 sent, every one bounced hard, under the floor
	warningDay    = 30 // Vandelay: 3 of 40 bounced hard, in the warning tier
)

const volumeFloor = 20

func daysAgo(days int) func() time.Time {
	return func() time.Time { return time.Now().Add(-time.Duration(days) * 24 * time.Hour) }
}

func TestASendingDomainInTheWarningTierIsADeliverabilityWarning(t *testing.T) {
	env := testhelper.Setup(t, testhelper.WithClock(daysAgo(warningDay)), testhelper.WithStaffVolumeFloor(volumeFloor))

	items := itemsOf(t, env, fixtures.VandelayID)

	warning, ok := itemOfKind(items, needsattentionreport.KindDeliverabilityWarning)
	require.True(t, ok)
	assert.Equal(t, needsattentionreport.SeverityDegraded, warning.Severity)
	require.NotNil(t, warning.Abuse)
	assert.Equal(t, fixtures.SendingDomainVandelayDomain, warning.Abuse.Domain)
	assert.Equal(t, abusewarning.MetricBounce, warning.Abuse.Metric)
	assert.InDelta(t, 3.0/40.0, warning.Abuse.Rate, 1e-9)
}

func TestTrafficUnderTheVolumeFloorIsNoDeliverabilityWarning(t *testing.T) {
	env := testhelper.Setup(t, testhelper.WithClock(daysAgo(belowFloorDay)), testhelper.WithStaffVolumeFloor(volumeFloor))

	items := itemsOf(t, env, fixtures.VandelayID)

	assert.NotContains(t, kinds(items), needsattentionreport.KindDeliverabilityWarning)
}

// The abuse detector is the licensed Edition's: without it nothing reads the warning tier.
func TestAnUnlicensedInstanceHasNoDeliverabilityWarning(t *testing.T) {
	env := testhelper.Setup(t, testhelper.WithoutLicense(), testhelper.WithClock(daysAgo(warningDay)), testhelper.WithStaffVolumeFloor(volumeFloor))

	items := itemsOf(t, env, fixtures.VandelayID)

	assert.NotContains(t, kinds(items), needsattentionreport.KindDeliverabilityWarning)
}
