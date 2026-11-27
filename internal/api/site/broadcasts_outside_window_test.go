package site_test

import (
	"net/http"
	"testing"
	"time"

	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	siteapi "github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// The editor asks, before Schedule, how many of an SMS Broadcast's recipients its Sending window
// would hold at the moment chosen (ADR 0134). Lacuna's four texters have Boston numbers; its window
// is narrowed to 8:00 to 20:00, so at 21:00 in New York all of them wait until 8:00 there.
func TestSiteOutsideWindowHoldsTheRecipientsOutsideTheirWindowUntilItOpens(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := t.Context()
	env.WindowSMS(t, fixtures.LacunaID, 8*time.Hour, 20*time.Hour)
	newYork := lo.Must(time.LoadLocation("America/New_York"))

	got, err := env.SiteActor(t, fixtures.LacunaOwnerLenaEmail).SiteBroadcastsOutsideWindow(ctx, &siteapi.SiteBroadcastOutsideWindowInput{
		Channel:     siteapi.BroadcastChannelSMS,
		ScheduledAt: siteapi.Timestamp(time.Date(2027, time.January, 15, 21, 0, 0, 0, newYork)),
	}, siteapi.SiteBroadcastsOutsideWindowParams{WorkspaceId: fixtures.LacunaPublicID})
	require.NoError(t, err)

	assert.Equal(t, int32(4), got.Held)
	reached, ok := got.ReachedAt.Get()
	require.True(t, ok)
	assert.WithinDuration(t, time.Date(2027, time.January, 16, 8, 0, 0, 0, newYork), time.Time(reached), 0)
}

// No window holds email.
func TestSiteOutsideWindowHoldsNoneOfAnEmailBroadcast(t *testing.T) {
	env := testhelper.Setup(t)

	got, err := env.SiteActor(t, fixtures.OwnerJohnEmail).SiteBroadcastsOutsideWindow(t.Context(), &siteapi.SiteBroadcastOutsideWindowInput{
		Channel:     siteapi.BroadcastChannelEmail,
		ScheduledAt: siteapi.Timestamp(time.Date(2027, time.January, 16, 3, 0, 0, 0, time.UTC)),
	}, siteapi.SiteBroadcastsOutsideWindowParams{WorkspaceId: fixtures.AcmePublicID})
	require.NoError(t, err)

	assert.Zero(t, got.Held)
	assert.False(t, got.ReachedAt.Set)
}

// The audience is the segment preview's: a condition that does not fit its field (a text test on
// Dunder's number field seats) cannot be counted, and is refused as the preview refuses it.
func TestSiteOutsideWindowRefusesAConditionThatDoesNotCompile(t *testing.T) {
	env := testhelper.Setup(t)

	_, err := env.SiteActor(t, fixtures.DunderOwnerDwightEmail).SiteBroadcastsOutsideWindow(t.Context(), &siteapi.SiteBroadcastOutsideWindowInput{
		Condition: siteapi.NewOptSegmentConditionInput(testhelper.ConditionInput[siteapi.SegmentConditionInput](t,
			`{"kind":"group","combinator":"and","entries":[{"kind":"customField","key":"seats","test":{"kind":"text","operator":"contains","value":"1"}}]}`)),
		Channel:     siteapi.BroadcastChannelSMS,
		ScheduledAt: siteapi.Timestamp(time.Date(2027, time.January, 15, 21, 0, 0, 0, time.UTC)),
	}, siteapi.SiteBroadcastsOutsideWindowParams{WorkspaceId: fixtures.DunderPublicID})

	testhelper.SiteRefusal(t, err, http.StatusUnprocessableEntity)
}
