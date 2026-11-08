package jobs_test

import (
	"testing"
	"time"

	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/broadcast"
	"github.com/getsphericon/sphericon/ent/broadcastrecipient"
	"github.com/getsphericon/sphericon/ent/outboundmessage"
	"github.com/getsphericon/sphericon/internal/domain/contacts/eligibility"
	"github.com/getsphericon/sphericon/internal/domain/orchestration/broadcasts"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/infra/events"
	"github.com/getsphericon/sphericon/internal/infra/messaging/twilio/twiliotest"
	"github.com/getsphericon/sphericon/internal/jobs"
	"github.com/getsphericon/sphericon/internal/kernel/channels"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// An SMS Broadcast (ADR 0080) goes to the Contacts whose phone passes SMS Send-eligibility (a
// phone, a Confirmation, no opt-out, no Suppression), one text per Contact through the Twilio account it names;
// eligibility is checked again at each send. Twilio is the fake REST API.

// Lacuna's Segment Textham holds Bea's confirmed phone, Uma's without a Confirmation and Sal's
// suppressed one.
func TestAnSMSBroadcastTextsOnlyTheConfirmedNumbers(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := t.Context()
	account := env.LacunaTwilio(t)
	id := int64(fixtures.BroadcastLacunaSmsGreetingID)

	require.NoError(t, jobs.SendBroadcast(ctx, env.DB, env.OutboundModule(), id))

	recipients := env.DB.BroadcastRecipient.Query().Where(broadcastrecipient.BroadcastID(id)).AllX(ctx)
	frozen := lo.Map(recipients, func(r *ent.BroadcastRecipient, _ int) int64 { return r.ContactID })
	assert.Equal(t, []int64{fixtures.ContactLacunaBeaID}, frozen, "a phone without a Confirmation, or a suppressed one, is not frozen")
	sent := env.Twilio.Messages()
	require.Len(t, sent, 1)
	assert.Equal(t, fixtures.ContactLacunaBeaPhone, sent[0].To)
	assert.Equal(t, "Hi Bea & co, <sale> today", sent[0].Body)
	assert.Equal(t, account.Integration.HookUrl, sent[0].StatusCallback)
	b := env.DB.Broadcast.GetX(ctx, id)
	assert.Equal(t, broadcast.StatusSent, b.Status)
	msg := env.DB.OutboundMessage.Query().Where(outboundmessage.BroadcastID(id)).OnlyX(ctx)
	assert.Equal(t, channels.SMS, msg.Channel)
	assert.Equal(t, sent[0].SID, lo.FromPtr(msg.ProviderMessageID))
	smsSent := env.QueuedCount(t, events.NameSmsSent, map[string]any{"broadcastId": id, "contactId": fixtures.ContactLacunaBeaID})
	assert.Equal(t, 1, smsSent)
	figures := figuresOf(t, env, id)
	assert.Equal(t, broadcasts.Stats{Recipients: 1, Sent: 1}, figures)
}

// A text of the Broadcast that Twilio refuses with 21610 (the number texted STOP to the sender
// there) is the number's opt-out, and its sms.unsubscribed names the Broadcast, which counts it
// when the event's consumer runs (ADR 0113). Lacuna's Segment Textham freezes Bea alone.
func TestAnSMSBroadcastsTextTwilioRefusesAsOptedOutCountsAsItsUnsubscribe(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := t.Context()
	env.LacunaTwilio(t)
	id := int64(fixtures.BroadcastLacunaSmsGreetingID)
	env.Twilio.RefuseNextSend(twiliotest.Unsubscribed)

	require.NoError(t, jobs.SendBroadcast(ctx, env.DB, env.OutboundModule(), id))
	before := figuresOf(t, env, id)
	env.PerformEnqueuedJobs(t, jobs.BroadcastUnsubscribeEventArgs{})
	after := figuresOf(t, env, id)

	stops := env.QueuedCount(t, events.NameSmsUnsubscribed, map[string]any{
		"broadcastId": id, "contactId": fixtures.ContactLacunaBeaID, "via": events.UnsubscribedByRefusedSend,
	})
	assert.Equal(t, 1, stops)
	assert.Equal(t, broadcasts.Stats{Recipients: 1, Skipped: 1}, before, "the send's transaction leaves the count to the event")
	assert.Equal(t, broadcasts.Stats{Recipients: 1, Skipped: 1, Unsubscribed: 1}, after)
}

// A Recipient frozen with a confirmed phone is checked again when its text is sent: one that lost
// its phone, opted out or was suppressed since is Skipped with that reason, and the figures add up.
// Lacuna's four confirmed phones are frozen: Ada's is cleared, the parked opt-out moves onto Bea's
// and Cyd's lifted Suppression is re-opened before the sends.
func TestAnSMSBroadcastSkipsARecipientThatLostItsPhoneOrOptedOutWithTheReason(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := t.Context()
	env.LacunaTwilio(t)
	id := int64(fixtures.BroadcastLacunaSmsHelloID)
	mod := env.OutboundModule()

	ids, err := planAudience(ctx, env.DB, mod, id)
	require.NoError(t, err)
	env.DB.Contact.UpdateOneID(fixtures.ContactLacunaAdaID).ClearPhone().ExecX(ctx)
	env.DB.Unsubscribe.UpdateOneID(fixtures.UnsubscribeLacunaParkedID).SetDestination(fixtures.ContactLacunaBeaPhone).ExecX(ctx)
	env.DB.Suppression.UpdateOneID(fixtures.SuppressionLacunaCydLiftedID).ClearLiftedAt().ExecX(ctx)
	for _, rid := range ids {
		require.NoError(t, deliverRecipient(ctx, env.DB, mod, rid))
	}

	assert.Len(t, ids, 4)
	sent := env.Twilio.Messages()
	texted := lo.Map(sent, func(m twiliotest.Message, _ int) string { return m.To })
	assert.Equal(t, []string{fixtures.ContactLacunaDotPhone}, texted)
	skipped := env.DB.OutboundMessage.Query().Where(outboundmessage.BroadcastID(id), outboundmessage.StatusEQ(outboundmessage.StatusSkipped)).AllX(ctx)
	reasons := lo.Associate(skipped, func(m *ent.OutboundMessage) (int64, string) {
		return lo.FromPtr(m.ContactID), lo.FromPtr(m.Reason)
	})
	assert.Equal(t, map[int64]string{
		fixtures.ContactLacunaAdaID: eligibility.ReasonNoDestination,
		fixtures.ContactLacunaBeaID: eligibility.ReasonUnsubscribedEverything,
		fixtures.ContactLacunaCydID: eligibility.ReasonSuppressed,
	}, reasons)
	figures := figuresOf(t, env, id)
	assert.Equal(t, broadcasts.Stats{Recipients: 4, Sent: 1, Skipped: 3}, figures)
}

// Lacuna's four confirmed phones are paced.
func TestAnSMSBroadcastPacesAtTheProviderLimit(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := t.Context()
	account := env.LacunaTwilio(t)
	env.DB.Integration.UpdateOneID(int64(account.Integration.ID)).SetMaxPerSecond(4).ExecX(ctx)
	id := int64(fixtures.BroadcastLacunaSmsHelloID)
	del := broadcasts.NewDelivery(env.OutboundModule())
	s := env.DB.Scoped(fixtures.LacunaID)
	start := time.Now()

	ids, err := del.Plan(ctx, s, id)
	require.NoError(t, err)
	times, err := del.Pace(ctx, s, id, ids, start)
	require.NoError(t, err)

	require.Len(t, times, 4)
	assert.Equal(t, 250*time.Millisecond, times[1].Sub(times[0]), "four a second is 250ms apart")
	assert.Equal(t, 250*time.Millisecond, times[2].Sub(times[1]))
	assert.Equal(t, 250*time.Millisecond, times[3].Sub(times[2]))
}
