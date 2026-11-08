package unsubscriberecording_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent/broadcastrecipient"
	"github.com/getsphericon/sphericon/internal/domain/contacts/eligibility"
	"github.com/getsphericon/sphericon/internal/domain/contacts/unsubscribes"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/infra/events"
	"github.com/getsphericon/sphericon/internal/jobs"
	"github.com/getsphericon/sphericon/internal/kernel/channels"
	"github.com/getsphericon/sphericon/internal/kernel/linktarget"
	"github.com/getsphericon/sphericon/internal/testhelper"
	"github.com/getsphericon/sphericon/internal/usecase/unsubscriberecording"
)

// The Broadcast a link names counts its recipient's opt-out from the opt-out's event, on every
// Channel (ADR 0113): the link's transaction writes the opt-out and inserts the broadcasts
// consumer's job, and working that job sets the recipient's mark, once. The parked recipients of
// the archived Broadcast are Alice, Bob and Carol.

// recordLink records the click on an unsubscribe link of Acme.
func recordLink(t *testing.T, env *testhelper.TestEnv, target linktarget.Unsubscribe) {
	t.Helper()
	err := unsubscriberecording.New(env.Bus, time.Now).RecordLink(t.Context(), env.DB.Scoped(fixtures.AcmeID), target)
	require.NoError(t, err)
}

// unsubscribedAt is the unsubscribe mark of the Broadcast recipient, nil while it is not counted.
func unsubscribedAt(t *testing.T, env *testhelper.TestEnv, recipientID int64) *time.Time {
	t.Helper()
	return env.DB.BroadcastRecipient.GetX(t.Context(), recipientID).UnsubscribedAt
}

func TestAnEmailLinkCountsOnItsBroadcastWhenTheEventsConsumerRuns(t *testing.T) {
	env := testhelper.Setup(t)
	recordLink(t, env, linktarget.Unsubscribe{
		Channel: channels.Email, Source: eligibility.SourceBroadcasts, Destination: fixtures.ContactAliceEmail,
		WorkspaceID: fixtures.AcmeID, ContactID: fixtures.ContactAliceID, BroadcastID: fixtures.BroadcastArchivedID,
	})
	before := unsubscribedAt(t, env, fixtures.BroadcastRecipientParkedAliceID)

	env.PerformEnqueuedJobs(t, jobs.BroadcastUnsubscribeEventArgs{})

	after := unsubscribedAt(t, env, fixtures.BroadcastRecipientParkedAliceID)
	assert.Nil(t, before, "the link's transaction leaves the count to the event")
	assert.NotNil(t, after)
}

func TestAnSMSLinkCountsOnItsBroadcastWhenTheEventsConsumerRuns(t *testing.T) {
	env := testhelper.Setup(t)
	const number = "+14155550133"
	env.DB.Contact.UpdateOneID(fixtures.ContactCarolID).SetPhone(number).ExecX(t.Context())
	recordLink(t, env, linktarget.Unsubscribe{
		Channel: channels.SMS, Source: eligibility.SourceBroadcasts, Destination: number,
		WorkspaceID: fixtures.AcmeID, ContactID: fixtures.ContactCarolID, BroadcastID: fixtures.BroadcastArchivedID,
	})
	before := unsubscribedAt(t, env, fixtures.BroadcastRecipientParkedCarolID)
	recorded := env.QueuedCount(t, events.NameSmsUnsubscribed, map[string]any{
		"contactId": fixtures.ContactCarolID, "broadcastId": fixtures.BroadcastArchivedID, "via": events.UnsubscribedByLink, "phone": number,
	})

	env.PerformEnqueuedJobs(t, jobs.BroadcastUnsubscribeEventArgs{})

	after := unsubscribedAt(t, env, fixtures.BroadcastRecipientParkedCarolID)
	assert.Equal(t, 1, recorded, "the link records sms.unsubscribed, naming the Broadcast")
	assert.Nil(t, before, "the link's transaction leaves the count to the event")
	assert.NotNil(t, after)
}

// A Telegram link was counted twice before: in its transaction and by the consumer of its event.
// Now only the event counts it, and a second click records nothing more.
func TestATelegramLinkCountsOnItsBroadcastOnceFromTheEvent(t *testing.T) {
	env := testhelper.Setup(t)
	target := linktarget.Unsubscribe{
		Channel: channels.Telegram, Source: eligibility.SourceBroadcasts, Destination: "424242",
		WorkspaceID: fixtures.AcmeID, ContactID: fixtures.ContactBobID, BroadcastID: fixtures.BroadcastArchivedID,
	}
	recordLink(t, env, target)
	before := unsubscribedAt(t, env, fixtures.BroadcastRecipientParkedBobID)

	env.PerformEnqueuedJobs(t, jobs.BroadcastUnsubscribeEventArgs{})
	counted := unsubscribedAt(t, env, fixtures.BroadcastRecipientParkedBobID)
	recordLink(t, env, target)
	env.PerformEnqueuedJobs(t, jobs.BroadcastUnsubscribeEventArgs{})
	again := unsubscribedAt(t, env, fixtures.BroadcastRecipientParkedBobID)

	assert.Nil(t, before, "the link's transaction leaves the count to the event")
	require.NotNil(t, counted)
	require.NotNil(t, again)
	assert.True(t, counted.Equal(*again), "counted once")
	recorded := env.QueuedCount(t, events.NameTelegramUnsubscribed, map[string]any{"broadcastId": fixtures.BroadcastArchivedID})
	assert.Equal(t, 1, recorded, "a second click records no second event")
}

// An opt-out of everything by a Telegram link is lifted by a `/start` (unsubscribes.LiftTelegram);
// a link of another Broadcast clicked after it is a new opt-out, which that Broadcast counts. The
// unengaged Contact 105 is a recipient of the sent Broadcast and of the one sent before it.
func TestATelegramLinkClickedAgainAfterAStartCountsOnItsBroadcast(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := t.Context()
	first := env.DB.BroadcastRecipient.GetX(ctx, fixtures.BroadcastRecipientUnengagedFirstID)
	earlier := env.DB.BroadcastRecipient.Query().Where(
		broadcastrecipient.ContactID(first.ContactID), broadcastrecipient.BroadcastIDNEQ(first.BroadcastID),
	).FirstX(ctx)
	const user = "434343"
	link := func(broadcastID int64) linktarget.Unsubscribe {
		return linktarget.Unsubscribe{
			Channel: channels.Telegram, Source: eligibility.SourceEverything, Destination: user,
			WorkspaceID: fixtures.AcmeID, ContactID: first.ContactID, BroadcastID: broadcastID,
		}
	}
	recordLink(t, env, link(first.BroadcastID))
	env.PerformEnqueuedJobs(t, jobs.BroadcastUnsubscribeEventArgs{})
	require.NoError(t, unsubscribes.LiftTelegram(ctx, env.DB.Scoped(fixtures.AcmeID), user))

	recordLink(t, env, link(earlier.BroadcastID))
	env.PerformEnqueuedJobs(t, jobs.BroadcastUnsubscribeEventArgs{})

	counted := unsubscribedAt(t, env, earlier.ID)
	assert.NotNil(t, counted, "the second opt-out counts on its own Broadcast")
	recorded := env.QueuedCount(t, events.NameTelegramUnsubscribed, nil)
	assert.Equal(t, 2, recorded, "each opt-out records its event")
}
