package jobs_test

import (
	"testing"

	"github.com/go-faster/jx"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	externalapi "github.com/getsphericon/sphericon/gen/external"
	siteapi "github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/domain/identity/apitokens"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/jobs"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// A refusal reaches back on SMS and Telegram as on email (ADR 0120): the clicks recorded of the
// Destination's earlier messages leave the Broadcast's figures, the messages count as untracked,
// and an Order credited through one of the clicks has no credit.

// lacunaStats is a Lacuna Broadcast's figures as its Owner reads them, once the stamps' projection
// ran.
func lacunaStats(t *testing.T, env *testhelper.TestEnv, id int64) siteapi.SiteBroadcastStats {
	t.Helper()
	env.PerformEnqueuedJobs(t, jobs.MessageObservationArgs{})
	lena := env.SiteActor(t, fixtures.LacunaOwnerLenaEmail)
	b, err := lena.SiteBroadcastsGet(t.Context(), siteapi.SiteBroadcastsGetParams{WorkspaceId: fixtures.LacunaPublicID, ID: siteapi.EntityId(id)})
	require.NoError(t, err)
	return b.Stats
}

// lacunaOrder places an Order Completed of the Contact with the phone number through /api, as a
// shop does, and works it into its Order.
func lacunaOrder(t *testing.T, env *testhelper.TestEnv, phone, orderID string) {
	t.Helper()
	c := env.ExternalWithToken(t, env.ScopedBearerFor(t, fixtures.LacunaID, apitokens.ScopeEventsWrite))
	err := c.EventsCreate(t.Context(), &externalapi.RecordEventsInput{Events: []externalapi.EventInput{{
		SubjectId: "shopper-" + orderID,
		Phone:     externalapi.NewOptNilString(phone),
		Action:    "Order Completed",
		Properties: externalapi.NewOptNilJsonObject(externalapi.JsonObject{
			"order_id": jx.Raw(`"` + orderID + `"`), "total": jx.Raw(`20`), "currency": jx.Raw(`"USD"`),
		}),
	}}})
	require.NoError(t, err)
	env.PerformEnqueuedJobs(t, jobs.OrderEventArgs{})
}

// refuseInLacuna records a refusal of the Lacuna Destination through /api and works the reach-back
// it starts.
func refuseInLacuna(t *testing.T, env *testhelper.TestEnv, channel externalapi.EngagementTrackingChannel, destination string) {
	t.Helper()
	lacunaTracking(t, env, channel, destination, externalapi.EngagementTrackingStateDenied)
	env.PerformEnqueuedJobs(t, jobs.TrackingWithdrawalArgs{})
}

// An SMS clicked, and an Order credited through the click, then a refusal of its number: the click
// leaves the Broadcast's figures, the message counts as untracked and the Order has no credit.
func TestARefusalDeletesTheClicksAndCreditOfTheNumbersEarlierSMS(t *testing.T) {
	env := testhelper.Setup(t)
	env.LacunaTwilio(t)
	require.NoError(t, env.SendBroadcastThrough(t, env.OutboundModule(), fixtures.BroadcastLacunaSmsLinksID))
	paths := smsShortPaths(t, env)
	require.Len(t, paths, 2)
	follow(t, env, paths[1], browserUA)
	lacunaOrder(t, env, fixtures.ContactLacunaBeaPhone, "S-1")
	before := lacunaStats(t, env, fixtures.BroadcastLacunaSmsLinksID)
	require.Equal(t, int32(1), before.ClickedCount, "the click was counted")
	require.Equal(t, int32(1), before.Revenue.Orders, "the Order was credited through the click")

	refuseInLacuna(t, env, externalapi.EngagementTrackingChannelSMS, fixtures.ContactLacunaBeaPhone)

	after := lacunaStats(t, env, fixtures.BroadcastLacunaSmsLinksID)
	assert.Zero(t, after.ClickedCount, "the click is gone")
	assert.Equal(t, before.UntrackedCount+1, after.UntrackedCount, "the message counts as untracked")
	assert.Zero(t, after.Revenue.Orders, "the Order has no credit")
}

// benPhone is the number Lacuna's Owner gives Ben, so a shop's Order names him.
const benPhone = "+16175557751"

// A Telegram message clicked, and an Order credited through the click, then a refusal of its
// chat: the click leaves the Broadcast's figures, the message counts as untracked and the Order has
// no credit; another chat's click stays.
func TestARefusalDeletesTheClicksAndCreditOfTheChatsEarlierTelegramMessage(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := t.Context()
	env.LacunaTelegram(t)
	lena := env.SiteActor(t, fixtures.LacunaOwnerLenaEmail)
	_, err := lena.SiteContactsUpdate(ctx, &siteapi.SiteUpdateContactInput{
		Revision: siteapi.Revision(env.ContactRevision(ctx, t, fixtures.ContactLacunaBenID)),
		Phone:    siteapi.NewOptNilString(benPhone),
	}, siteapi.SiteContactsUpdateParams{WorkspaceId: fixtures.LacunaPublicID, ID: siteapi.EntityId(fixtures.ContactLacunaBenID)})
	require.NoError(t, err)
	require.NoError(t, env.SendBroadcastThrough(t, env.OutboundModule(), fixtures.BroadcastLacunaTelegramClicksID))
	bens, _ := shortPaths(t, env, benChat)
	fays, _ := shortPaths(t, env, fayChat)
	require.Len(t, bens, 1)
	require.Len(t, fays, 1)
	follow(t, env, bens[0], browserUA)
	follow(t, env, fays[0], browserUA)
	lacunaOrder(t, env, benPhone, "T-1")
	before := lacunaStats(t, env, fixtures.BroadcastLacunaTelegramClicksID)
	require.Equal(t, int32(2), before.ClickedCount, "both clicks were counted")
	require.Equal(t, int32(1), before.Revenue.Orders, "the Order was credited through Ben's click")

	refuseInLacuna(t, env, externalapi.EngagementTrackingChannelTelegram, benChat)

	after := lacunaStats(t, env, fixtures.BroadcastLacunaTelegramClicksID)
	assert.Equal(t, int32(1), after.ClickedCount, "the refusing chat's click is gone, the other stays")
	assert.Equal(t, before.UntrackedCount+1, after.UntrackedCount, "the refusing chat's message counts as untracked")
	assert.Zero(t, after.Revenue.Orders, "the Order has no credit")
}

// A Telegram message to a Contact's two chats, clicked, then a refusal of the chat it reached
// second: the refusal reaches the message all the same (ADR 0120), so the click leaves the
// Broadcast's figures, the message counts as untracked and a later click records nothing.
func TestARefusalOfEitherChatReachesATelegramMessageToTwo(t *testing.T) {
	env := testhelper.Setup(t)
	env.LacunaTelegram(t)
	require.NoError(t, env.SendBroadcastThrough(t, env.OutboundModule(), fixtures.BroadcastLacunaTelegramClicksID))
	tias, _ := shortPaths(t, env, tiaOtherChat)
	require.Len(t, tias, 1)
	follow(t, env, tias[0], browserUA)
	before := lacunaStats(t, env, fixtures.BroadcastLacunaTelegramClicksID)
	require.Equal(t, int32(1), before.ClickedCount, "Tia's click was counted")

	refuseInLacuna(t, env, externalapi.EngagementTrackingChannelTelegram, tiaOtherChat)
	follow(t, env, tias[0], browserUA)

	after := lacunaStats(t, env, fixtures.BroadcastLacunaTelegramClicksID)
	assert.Zero(t, after.ClickedCount, "the click is gone and the later one recorded nothing")
	assert.Equal(t, before.UntrackedCount+1, after.UntrackedCount, "the message counts as untracked")
}

// A refusal, then an Erasure of the Contact before its reach-back has run: what the refusal
// deletes is gone all the same (ADR 0113: the outcome does not depend on the order), though the
// Erasure blanked the Destination the reach-back finds the messages by. The click leaves the
// Broadcast's figures, the message counts as untracked and the Order has no credit.
func TestAnErasureBeforeTheReachBackLeavesNothingTheRefusalDeletes(t *testing.T) {
	env := testhelper.Setup(t)
	env.LacunaTwilio(t)
	require.NoError(t, env.SendBroadcastThrough(t, env.OutboundModule(), fixtures.BroadcastLacunaSmsLinksID))
	paths := smsShortPaths(t, env)
	require.Len(t, paths, 2)
	follow(t, env, paths[1], browserUA)
	lacunaOrder(t, env, fixtures.ContactLacunaBeaPhone, "S-2")
	before := lacunaStats(t, env, fixtures.BroadcastLacunaSmsLinksID)
	require.Equal(t, int32(1), before.ClickedCount, "the click was counted")
	require.Equal(t, int32(1), before.Revenue.Orders, "the Order was credited through the click")

	lacunaTracking(t, env, externalapi.EngagementTrackingChannelSMS, fixtures.ContactLacunaBeaPhone, externalapi.EngagementTrackingStateDenied)
	eraser := env.ExternalWithToken(t, env.ScopedBearerFor(t, fixtures.LacunaID, apitokens.ScopeContactsErase))
	require.NoError(t, eraser.ContactsDelete(t.Context(), externalapi.ContactsDeleteParams{ID: externalapi.EntityId(fixtures.ContactLacunaBeaID)}))
	env.PerformEnqueuedJobs(t, jobs.TrackingWithdrawalArgs{})

	after := lacunaStats(t, env, fixtures.BroadcastLacunaSmsLinksID)
	assert.Zero(t, after.ClickedCount, "the click is gone")
	assert.Equal(t, before.UntrackedCount+1, after.UntrackedCount, "the message counts as untracked")
	assert.Zero(t, after.Revenue.Orders, "the Order has no credit")
}
