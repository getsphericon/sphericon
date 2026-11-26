package jobs_test

import (
	"fmt"
	"testing"
	"time"

	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/broadcast"
	"github.com/getsphericon/sphericon/ent/broadcastrecipient"
	"github.com/getsphericon/sphericon/ent/outboundmessage"
	"github.com/getsphericon/sphericon/internal/domain/orchestration/broadcasts"
	"github.com/getsphericon/sphericon/internal/domain/sending/outboundmessages"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/infra/messaging/twilio/twiliotest"
	"github.com/getsphericon/sphericon/internal/kernel/channels"
	"github.com/getsphericon/sphericon/internal/kernel/skipreason"
	"github.com/getsphericon/sphericon/internal/kernel/systemevent"
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

	require.NoError(t, env.SendBroadcastThrough(t, env.OutboundModule(), id))

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
	smsSent := env.QueuedCount(t, systemevent.SmsSent, map[string]any{"broadcastId": id, "contactId": fixtures.ContactLacunaBeaID})
	assert.Equal(t, 1, smsSent)
	figures := figuresOf(t, env, id)
	assert.Equal(t, broadcasts.Stats{Recipients: 1, Sent: 1, SingleZone: 1}, figures, "a Boston number is judged by one zone")
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
	reasons := lo.Associate(skipped, func(m *ent.OutboundMessage) (int64, skipreason.Reason) {
		return lo.FromPtr(m.ContactID), lo.FromPtr(m.SkippedReason)
	})
	assert.Equal(t, map[int64]skipreason.Reason{
		fixtures.ContactLacunaAdaID: skipreason.NoDestination,
		fixtures.ContactLacunaBeaID: skipreason.UnsubscribedEverything,
		fixtures.ContactLacunaCydID: skipreason.Suppressed,
	}, reasons)
	figures := figuresOf(t, env, id)
	assert.Equal(t, broadcasts.Stats{Recipients: 4, Sent: 1, SingleZone: 1, Skipped: 3, SkippedBy: map[skipreason.Reason]int{
		skipreason.NoDestination: 1, skipreason.UnsubscribedEverything: 1, skipreason.Suppressed: 1,
	}}, figures)
}

// Bea's number got three Broadcast texts in the last day, at Lacuna's cap of three a day: the
// Greeting's Recipient for her is Skipped with frequency_cap, not texted (ADR 0137).
func TestAnSMSBroadcastSkipsARecipientOverItsFrequencyCap(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := t.Context()
	account := env.LacunaTwilio(t)
	bea := env.DB.Contact.GetX(ctx, fixtures.ContactLacunaBeaID)
	integrationID := int64(account.Integration.ID)
	for i := range 3 {
		res, err := env.OutboundModule().Send(ctx, env.DB.Scoped(fixtures.LacunaID), outboundmessages.Request{
			Ref:     outboundmessages.BroadcastRef{BroadcastID: fixtures.BroadcastLacunaSmsHelloID},
			Key:     fmt.Sprintf("earlier:%d", i),
			Contact: bea, Destination: *bea.Phone, IntegrationID: &integrationID,
			Content: outboundmessages.SMSContent{Text: "Earlier news"},
		})
		require.NoError(t, err)
		require.Equal(t, outboundmessages.Sent, res.Outcome)
	}
	id := int64(fixtures.BroadcastLacunaSmsGreetingID)

	require.NoError(t, env.SendBroadcastThrough(t, env.OutboundModule(), id))

	assert.Len(t, env.Twilio.Messages(), 3, "only the earlier texts went out")
	msg := env.DB.OutboundMessage.Query().Where(outboundmessage.BroadcastID(id)).OnlyX(ctx)
	assert.Equal(t, outboundmessage.StatusSkipped, msg.Status)
	assert.Equal(t, skipreason.FrequencyCap, lo.FromPtr(msg.SkippedReason))
	figures := figuresOf(t, env, id)
	assert.Equal(t, broadcasts.Stats{Recipients: 1, Skipped: 1, SkippedBy: map[skipreason.Reason]int{skipreason.FrequencyCap: 1}}, figures)
}

// Lacuna's four confirmed phones are paced.
func TestAnSMSBroadcastPacesAtTheProviderLimit(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := t.Context()
	account := env.LacunaTwilio(t)
	env.DB.Integration.UpdateOneID(int64(account.Integration.ID)).SetMaxPerSecond(4).ExecX(ctx)
	id := int64(fixtures.BroadcastLacunaSmsHelloID)
	del := testhelper.Module[*broadcasts.Delivery](env)
	s := env.DB.Scoped(fixtures.LacunaID)

	ids, err := del.Plan(ctx, s, id)
	require.NoError(t, err)
	times, err := del.Pace(ctx, s, id, ids)
	require.NoError(t, err)

	require.Len(t, times, 4)
	assert.Equal(t, 250*time.Millisecond, times[1].Sub(times[0]), "four a second is 250ms apart")
	assert.Equal(t, 250*time.Millisecond, times[2].Sub(times[1]))
	assert.Equal(t, 250*time.Millisecond, times[3].Sub(times[2]))
}
