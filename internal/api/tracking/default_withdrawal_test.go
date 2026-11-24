package tracking_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	externalapi "github.com/getsphericon/sphericon/gen/external"
	siteapi "github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/domain/sending/outboundmessages"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/jobs"
	"github.com/getsphericon/sphericon/internal/kernel/systemevent"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// A default turned to dont_track withdraws the tracking of the messages already sent to every
// Destination without a granted, and deletes nothing (ADR 0120): they leave the rates, their
// recorded opens stay. A default turned back to track restores nothing and applies to the messages
// rendered from then on.

// withdraw works the queued withdrawals of tracking, as the job runner does.
func withdraw(t *testing.T, env *testhelper.TestEnv) {
	t.Helper()
	env.PerformEnqueuedJobs(t, outboundmessages.WithdrawUngrantedArgs{})
}

// globexOpensOf lists the opens recorded of address in Globex, as its Owner reads the Events.
func globexOpensOf(t *testing.T, env *testhelper.TestEnv, address string) []siteapi.SiteEventResource {
	t.Helper()
	jane := env.SiteActor(t, fixtures.OwnerJaneEmail)
	res, err := jane.SiteEventsList(t.Context(), siteapi.SiteEventsListParams{
		WorkspaceId: fixtures.GlobexPublicID,
		Action:      siteapi.NewOptString(string(systemevent.EmailOpened)),
		Email:       siteapi.NewOptString(address),
	})
	require.NoError(t, err)
	return res.Items
}

// Messages sent under track, then the default turned to dont_track: the message of the Destination
// with no consent counts as untracked in the report, the one of the Destination that granted stays
// tracked, and the open recorded of the withdrawn message is still there.
func TestADefaultTurnedToDontTrackWithdrawsTheMessagesOfDestinationsWithoutAGrant(t *testing.T) {
	env := testhelper.Setup(t)
	globexTracking(t, env, fixtures.ContactGlobexReaderEmail, externalapi.EngagementTrackingStateGranted)
	html := sendToReader(t, env)
	trackingGet(t, env, capturedPixel.FindString(html))
	trackingGet(t, env, openPath(t, env, fixtures.OutboundMessageGlobexUsageID))
	env.PerformEnqueuedJobs(t, jobs.MessageObservationArgs{})

	setGlobexDefault(t, env, siteapi.EngagementTrackingDecisionDontTrack)
	withdraw(t, env)

	email := globexOverview(t, env).Current.Email
	assert.Equal(t, int32(1), email.SentCount, "the granted reader's message stays tracked")
	assert.Equal(t, int32(1), email.UntrackedCount, "the usage message is withdrawn")
	assert.Equal(t, int32(1), email.OpenedCount, "the rates count the tracked message's open")
	kept := globexOpensOf(t, env, fixtures.OutboundMessageGlobexUsageDestination)
	assert.Len(t, kept, 1, "the open recorded of the withdrawn message is kept")
}

// A default turned back to track gives no message its tracking back.
func TestADefaultTurnedBackToTrackRestoresNothing(t *testing.T) {
	env := testhelper.Setup(t)
	setGlobexDefault(t, env, siteapi.EngagementTrackingDecisionDontTrack)
	withdraw(t, env)

	setGlobexDefault(t, env, siteapi.EngagementTrackingDecisionTrack)
	withdraw(t, env)

	email := globexOverview(t, env).Current.Email
	assert.Zero(t, email.SentCount)
	assert.Equal(t, int32(1), email.UntrackedCount, "the usage message stays withdrawn")
}

// A message sent after the default was turned back to track is tracked, even when the withdrawal of
// the earlier turn to dont_track runs only after it: the withdrawal reaches only what was there
// when the default was turned.
func TestAMessageSentAfterTheDefaultTurnedBackToTrackIsTracked(t *testing.T) {
	env := testhelper.Setup(t)
	setGlobexDefault(t, env, siteapi.EngagementTrackingDecisionDontTrack)
	setGlobexDefault(t, env, siteapi.EngagementTrackingDecisionTrack)
	html := sendToReader(t, env)
	require.NotEmpty(t, capturedPixel.FindString(html), "the new message went out tracked")

	withdraw(t, env)

	email := globexOverview(t, env).Current.Email
	assert.Equal(t, int32(1), email.SentCount, "the reader's new message is tracked")
	assert.Equal(t, int32(1), email.UntrackedCount, "the usage message, sent before the turn, is withdrawn")
}

// Turning the default to dont_track again withdraws nothing more than the first turn did.
func TestTurningTheDefaultToDontTrackAgainWithdrawsNothingMore(t *testing.T) {
	env := testhelper.Setup(t)
	globexTracking(t, env, fixtures.ContactGlobexReaderEmail, externalapi.EngagementTrackingStateGranted)
	sendToReader(t, env)
	setGlobexDefault(t, env, siteapi.EngagementTrackingDecisionDontTrack)
	withdraw(t, env)
	once := globexOverview(t, env).Current.Email

	setGlobexDefault(t, env, siteapi.EngagementTrackingDecisionTrack)
	setGlobexDefault(t, env, siteapi.EngagementTrackingDecisionDontTrack)
	withdraw(t, env)
	twice := globexOverview(t, env).Current.Email

	assert.Equal(t, int32(1), once.UntrackedCount)
	assert.Equal(t, once.SentCount, twice.SentCount)
	assert.Equal(t, once.UntrackedCount, twice.UntrackedCount)
}

// An open that arrives after the default turned to dont_track, before its withdrawal reached the
// message, records nothing: the recording reads the live default.
func TestAnOpenBeforeTheWithdrawalReachesTheMessageRecordsNothing(t *testing.T) {
	env := testhelper.Setup(t)
	setGlobexDefault(t, env, siteapi.EngagementTrackingDecisionDontTrack)

	trackingGet(t, env, openPath(t, env, fixtures.OutboundMessageGlobexUsageID))

	opens := env.QueuedEvents(t, systemevent.EmailOpened)
	assert.Empty(t, opens, "no open is recorded")
}
