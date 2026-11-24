package jobs_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	externalapi "github.com/getsphericon/sphericon/gen/external"
	siteapi "github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/domain/identity/apitokens"
	"github.com/getsphericon/sphericon/internal/domain/sending/outboundmessages"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// A default turned to dont_track withdraws the tracking of the messages already sent on every
// Channel (ADR 0120): an SMS to a number without a granted and the Telegram messages to the chats
// without one leave the tracked count, a granted chat's message stays.

// lacunaReport is the report of a Lacuna Broadcast, as the customer reads it through /api.
func lacunaReport(t *testing.T, env *testhelper.TestEnv, broadcastID int64) *externalapi.BroadcastReport {
	t.Helper()
	c := env.ExternalWithToken(t, env.ScopedBearerFor(t, fixtures.LacunaID, apitokens.ScopeBroadcastsRead))
	report, err := c.BroadcastsReport(t.Context(), externalapi.BroadcastsReportParams{ID: externalapi.EntityId(broadcastID)})
	require.NoError(t, err)
	return report
}

func TestADefaultTurnedToDontTrackWithdrawsSMSAndTelegramMessagesWithoutAGrant(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := t.Context()
	env.LacunaTwilio(t)
	env.LacunaTelegram(t)
	lacunaTracking(t, env, externalapi.EngagementTrackingChannelTelegram, fayChat, externalapi.EngagementTrackingStateGranted)
	require.NoError(t, env.SendBroadcastThrough(t, env.OutboundModule(), fixtures.BroadcastLacunaSmsLinksID))
	require.NoError(t, env.SendBroadcastThrough(t, env.OutboundModule(), fixtures.BroadcastLacunaTelegramClicksID))
	telegramBefore := lacunaReport(t, env, fixtures.BroadcastLacunaTelegramClicksID)
	require.Zero(t, telegramBefore.UntrackedCount, "every Telegram message went out tracked")
	require.Greater(t, telegramBefore.SentCount, int32(1), "the chat that granted is not the only one")

	lena := env.SiteActor(t, fixtures.LacunaOwnerLenaEmail)
	_, err := lena.SiteWorkspacesSetEngagementTrackingDefault(ctx, &siteapi.SiteSetEngagementTrackingDefaultInput{
		Revision:                  siteapi.Revision(env.WorkspaceRevision(ctx, t, fixtures.LacunaPublicID)),
		EngagementTrackingDefault: siteapi.EngagementTrackingDecisionDontTrack,
	}, siteapi.SiteWorkspacesSetEngagementTrackingDefaultParams{WorkspaceId: fixtures.LacunaPublicID})
	require.NoError(t, err)
	env.PerformEnqueuedJobs(t, outboundmessages.WithdrawUngrantedArgs{})

	sms := lacunaReport(t, env, fixtures.BroadcastLacunaSmsLinksID)
	telegram := lacunaReport(t, env, fixtures.BroadcastLacunaTelegramClicksID)
	assert.Equal(t, int32(1), sms.SentCount)
	assert.Equal(t, int32(1), sms.UntrackedCount, "the SMS to a number without a granted is withdrawn")
	assert.Equal(t, telegramBefore.SentCount-1, telegram.UntrackedCount, "every chat's message but the granted one is withdrawn")
}
