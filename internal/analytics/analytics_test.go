package analytics_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/internal/analytics"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

func overview(t *testing.T, ws int64, w analytics.Window) *analytics.Overview {
	t.Helper()
	env := testhelper.Setup(t)
	ov, err := analytics.New().Overview(context.Background(), env.DB.Scoped(ws), w)
	require.NoError(t, err)
	return ov
}

// Fixtures (broadcast_recipients.yml): Acme sent 11 (5 opened, 2 clicked) three days
// ago, 10 (4, 1) nine days ago, 10 (5, 2) sixteen days ago, 9 (4, 1) twenty-four days
// ago, 10 (6, 3) forty days ago and 10 (4, 1) seventy days ago.
func TestEngagementIsCountedOverTheSendCohortOfTheWindow(t *testing.T) {
	cases := []struct {
		window                analytics.Window
		sent, opened, clicked int
	}{
		{analytics.Window7Days, 11, 5, 2},
		{analytics.Window30Days, 40, 18, 6},
		{analytics.Window90Days, 60, 28, 10},
	}
	for _, c := range cases {
		ov := overview(t, fixtures.AcmeID, c.window)
		assert.Equal(t, c.sent, ov.Email.Sent, c.window)
		assert.Equal(t, c.opened, ov.Email.Opened, c.window)
		assert.Equal(t, c.clicked, ov.Email.Clicked, c.window)
		assert.InDelta(t, float64(c.opened)/float64(c.sent), ov.Email.OpenRate, 1e-6)
		assert.InDelta(t, float64(c.clicked)/float64(c.sent), ov.Email.ClickRate, 1e-6)
		assert.InDelta(t, float64(c.clicked)/float64(c.opened), ov.Email.ClickToOpenRate, 1e-6)
		assert.LessOrEqual(t, ov.Email.Opened, ov.Email.Sent)
		assert.LessOrEqual(t, ov.Email.Clicked, ov.Email.Opened)
		assert.LessOrEqual(t, ov.Email.OpenRate, float32(1))
	}
}

func TestTimeSeriesIsOneZeroFilledUTCDayPerDayEndingToday(t *testing.T) {
	ov := overview(t, fixtures.AcmeID, analytics.Window7Days)
	require.Len(t, ov.Series, 7)

	today := time.Now().UTC().Truncate(24 * time.Hour)
	sent := 0
	for i, p := range ov.Series {
		assert.Equal(t, today.AddDate(0, 0, i-6).Format("2006-01-02"), p.Date)
		sent += p.Sent
	}
	assert.Equal(t, ov.Email.Sent, sent, "the series reconciles with the cards")

	threeDaysAgo := ov.Series[3]
	assert.Equal(t, 11, threeDaysAgo.Sent)
	assert.Equal(t, 5, threeDaysAgo.Opened)
	assert.Equal(t, 2, threeDaysAgo.Clicked)
	assert.Equal(t, analytics.Point{Date: ov.Series[6].Date}, ov.Series[6], "an empty day is all zeros")
}

func TestSeriesLengthFollowsTheWindow(t *testing.T) {
	assert.Len(t, overview(t, fixtures.AcmeID, analytics.Window30Days).Series, 30)
	assert.Len(t, overview(t, fixtures.AcmeID, analytics.Window90Days).Series, 90)
}

func TestContactsSplitByTheGlobalOptOutPredicate(t *testing.T) {
	ov := overview(t, fixtures.AcmeID, analytics.Window30Days)
	assert.Positive(t, ov.Contacts.Total)
	assert.Positive(t, ov.Contacts.Unsubscribed, "suppressed and opted-out contacts are non-mailable")
	assert.Equal(t, ov.Contacts.Total, ov.Contacts.Active+ov.Contacts.Unsubscribed)
	assert.LessOrEqual(t, ov.Contacts.NewInWindow, ov.Contacts.Total)
}

func TestAutomationsSnapshot(t *testing.T) {
	ov := overview(t, fixtures.AcmeID, analytics.Window30Days)
	assert.Positive(t, ov.Automations.Total)
	assert.LessOrEqual(t, ov.Automations.Active, ov.Automations.Total)
}

func TestAnEmptyCohortHasZeroRates(t *testing.T) {
	ov := overview(t, fixtures.AcmeID, analytics.Window7Days)
	empty := ov.Series[6]
	require.Zero(t, empty.Sent)

	// A window nothing was sent in: no Workspace but Acme has recipients, so use the other.
	other := overview(t, fixtures.GlobexID, analytics.Window7Days)
	assert.Zero(t, other.Email.Sent)
	assert.Zero(t, other.Email.OpenRate)
	assert.Zero(t, other.Email.ClickToOpenRate)
}

// An archived Automation is out of the counts, active or not.
func TestArchivedAutomationsAreNotCounted(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	acme := env.DB.Scoped(fixtures.AcmeID)
	before, err := analytics.New().Overview(ctx, acme, analytics.Window30Days)
	require.NoError(t, err)
	require.NoError(t, acme.Automation().Discard(ctx, fixtures.AutomationWelcomeSeriesID))

	after, err := analytics.New().Overview(ctx, acme, analytics.Window30Days)
	require.NoError(t, err)
	assert.Equal(t, before.Automations.Total-1, after.Automations.Total)
	assert.Equal(t, before.Automations.Active-1, after.Automations.Active)
}
