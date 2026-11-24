package jobs_test

import (
	"strings"
	"testing"

	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/event"
	externalapi "github.com/getsphericon/sphericon/gen/external"
	"github.com/getsphericon/sphericon/internal/domain/identity/apitokens"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/infra/events"
	"github.com/getsphericon/sphericon/internal/jobs"
	"github.com/getsphericon/sphericon/internal/kernel/systemevent"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// An Erasure leaves nothing that names the erased person's Destinations (ADR 0021): no click Event
// keeps the number or the Telegram user its message went to, and no queued job, a refusal's
// reach-back or an event's, holds one.

// eraseInLacuna erases the Lacuna Contact through /api, as the customer does.
func eraseInLacuna(t *testing.T, env *testhelper.TestEnv, contactID int64) {
	t.Helper()
	eraser := env.ExternalWithToken(t, env.ScopedBearerFor(t, fixtures.LacunaID, apitokens.ScopeContactsErase))
	require.NoError(t, eraser.ContactsDelete(t.Context(), externalapi.ContactsDeleteParams{ID: externalapi.EntityId(contactID)}))
}

// queuedNaming is the queued jobs whose args name destination: the event jobs whose event names it
// (by their event's name) and the refusals' reach-backs of it.
func queuedNaming(t *testing.T, env *testhelper.TestEnv, destination string) []string {
	t.Helper()
	quoted := `"` + destination + `"`
	named := lo.FilterMap(env.QueuedEnvelopes(t), func(e events.Envelope, _ int) (string, bool) {
		return e.Name, strings.Contains(string(e.Data), quoted)
	})
	reachBacks := lo.FilterMap(env.JobsOf(t, jobs.TrackingWithdrawalArgs{}.Kind()), func(args map[string]any, _ int) (string, bool) {
		return "reach-back", args["destination"] == destination
	})
	return append(named, reachBacks...)
}

// clickTrace is what a click Event keeps of the Destination its message went to.
type clickTrace struct {
	Destination any
	Phone       *string
}

// clickTraces are Lacuna's stored click Events of action, as their Destination traces.
func clickTraces(t *testing.T, env *testhelper.TestEnv, action string) []clickTrace {
	t.Helper()
	clicks, err := env.DB.Scoped(fixtures.LacunaID).Event().Query().Where(event.Action(action)).Order(event.ByID()).All(t.Context())
	require.NoError(t, err)
	return lo.Map(clicks, func(e *ent.Event, _ int) clickTrace {
		return clickTrace{Destination: e.Properties["destination"], Phone: e.Phone}
	})
}

// A click of an SMS names the number the text went to; once its Contact is erased the click stays,
// a count with no number in its properties or its phone.
func TestAnErasureLeavesNoSMSClickThatNamesTheNumber(t *testing.T) {
	env := testhelper.Setup(t)
	env.LacunaTwilio(t)
	require.NoError(t, env.SendBroadcastThrough(t, env.OutboundModule(), fixtures.BroadcastLacunaSmsLinksID))
	paths := smsShortPaths(t, env)
	require.Len(t, paths, 2)
	follow(t, env, paths[1], browserUA)
	number := fixtures.ContactLacunaBeaPhone
	require.Equal(t, []clickTrace{{Destination: number, Phone: &number}}, clickTraces(t, env, systemevent.SmsClicked),
		"the click names the number")

	eraseInLacuna(t, env, fixtures.ContactLacunaBeaID)

	assert.Equal(t, []clickTrace{{}}, clickTraces(t, env, systemevent.SmsClicked), "the click keeps no number")
}

// A click of a Telegram message names the user the message went to; once its Contact is erased the
// click stays, a count with no user id.
func TestAnErasureLeavesNoTelegramClickThatNamesTheUser(t *testing.T) {
	env := testhelper.Setup(t)
	env.LacunaTelegram(t)
	require.NoError(t, env.SendBroadcastThrough(t, env.OutboundModule(), fixtures.BroadcastLacunaTelegramClicksID))
	tias, _ := shortPaths(t, env, tiaOtherChat)
	require.Len(t, tias, 1)
	follow(t, env, tias[0], browserUA)
	require.Equal(t, []clickTrace{{Destination: tiaChat}}, clickTraces(t, env, systemevent.TelegramClicked),
		"the click names the user the message went to")

	eraseInLacuna(t, env, fixtures.ContactLacunaTwoChatsID)

	assert.Equal(t, []clickTrace{{}}, clickTraces(t, env, systemevent.TelegramClicked), "the click keeps no user id")
}

// A refusal of a number queues its reach-back, which names the number; an Erasure of its Contact
// before the job runs does the job's work itself and leaves no queued job naming the number.
func TestAnErasureBeforeTheReachBackLeavesNoQueuedJobNamingTheNumber(t *testing.T) {
	env := testhelper.Setup(t)
	number := fixtures.ContactLacunaBeaPhone
	lacunaTracking(t, env, externalapi.EngagementTrackingChannelSMS, number, externalapi.EngagementTrackingStateDenied)
	require.Contains(t, queuedNaming(t, env, number), "reach-back", "the reach-back waits, naming the number")

	eraseInLacuna(t, env, fixtures.ContactLacunaBeaID)

	assert.Empty(t, queuedNaming(t, env, number), "no queued job names the number")
}

// The same of a refusal of one of a Contact's Telegram chats: the reach-back names the user id.
func TestAnErasureBeforeTheReachBackLeavesNoQueuedJobNamingTheTelegramUser(t *testing.T) {
	env := testhelper.Setup(t)
	lacunaTracking(t, env, externalapi.EngagementTrackingChannelTelegram, tiaOtherChat, externalapi.EngagementTrackingStateDenied)
	require.Contains(t, queuedNaming(t, env, tiaOtherChat), "reach-back", "the reach-back waits, naming the user")

	eraseInLacuna(t, env, fixtures.ContactLacunaTwoChatsID)

	assert.Empty(t, queuedNaming(t, env, tiaOtherChat), "no queued job names the user")
}

// Consent a number gave before any Contact had it names the number alone; once a Contact that took
// the number is erased, no queued job of that consent's Event names the number.
func TestAnErasureLeavesNoQueuedJobOfTheConsentItsNumberGaveBeforeItWasAContact(t *testing.T) {
	env := testhelper.Setup(t)
	const number = "+16175557790"
	lacunaTracking(t, env, externalapi.EngagementTrackingChannelSMS, number, externalapi.EngagementTrackingStateGranted)
	require.Contains(t, queuedNaming(t, env, number), systemevent.SmsEngagementTrackingGranted, "the consent's Event names the number")
	c := env.ExternalWithToken(t, env.ScopedBearerFor(t, fixtures.LacunaID, apitokens.ScopeContactsWrite))
	created, err := c.ContactsCreate(t.Context(), &externalapi.CreateContactInput{Phone: externalapi.NewOptNilString(number)})
	require.NoError(t, err)

	eraseInLacuna(t, env, int64(created.ID))

	assert.Empty(t, queuedNaming(t, env, number), "no queued job names the number")
}
