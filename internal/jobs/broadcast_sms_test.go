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
	externalapi "github.com/getsphericon/sphericon/gen/external"
	"github.com/getsphericon/sphericon/internal/broadcasts"
	"github.com/getsphericon/sphericon/internal/eligibility"
	"github.com/getsphericon/sphericon/internal/events"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/jobs"
	"github.com/getsphericon/sphericon/internal/messaging/twilio/twiliotest"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// An SMS Broadcast (ADR 0080) goes to the Contacts whose phone passes SMS Send-eligibility (a
// phone, a Confirmation, no opt-out, no Suppression), one text per Contact through the Twilio account it names;
// eligibility is checked again at each send. Twilio is the fake REST API.

// alicePhone and bobPhone are the numbers a test gives Alice and Bob, who have none in the fixtures.
const (
	alicePhone = "+14155550901"
	bobPhone   = "+14155550902"
)

// smsBroadcast creates a draft SMS Broadcast to all contacts through the account with text.
func smsBroadcast(t *testing.T, env *testhelper.TestEnv, account testhelper.TwilioAccount, text string) int64 {
	t.Helper()
	b, err := broadcasts.New(env.Bus, testhelper.Renderer(), env.OutboundModule()).Create(t.Context(), env.DB.Scoped(fixtures.AcmeID), broadcasts.CreateInput{
		Name:          new("SMS news"),
		Content:       broadcasts.SMSContentInput{Text: &text},
		IntegrationID: new(int64(account.Integration.ID)),
	})
	require.NoError(t, err)
	return b.ID
}

func TestAnSMSBroadcastTextsOnlyTheConfirmedNumbers(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := t.Context()
	account := env.ConnectTwilio(t)
	confirmSMS(t, env, fixtures.ContactTexterPhone)
	env.DB.Contact.UpdateOneID(fixtures.ContactAliceID).SetPhone(alicePhone).ExecX(ctx)
	confirmSMS(t, env, alicePhone)
	env.SuppressNumber(ctx, alicePhone)
	id := smsBroadcast(t, env, account, "Hi {{ first_name }} & co, <sale> today")

	require.NoError(t, jobs.SendBroadcast(ctx, env.DB, env.OutboundModule(), id))

	recipients := env.DB.BroadcastRecipient.Query().Where(broadcastrecipient.BroadcastID(id)).AllX(ctx)
	frozen := lo.Map(recipients, func(r *ent.BroadcastRecipient, _ int) int64 { return r.ContactID })
	assert.Equal(t, []int64{fixtures.ContactTexterID}, frozen, "a phone without a Confirmation, or a suppressed one, is not frozen")
	sent := env.Twilio.Messages()
	require.Len(t, sent, 1)
	assert.Equal(t, fixtures.ContactTexterPhone, sent[0].To)
	assert.Equal(t, "Hi Tess & co, <sale> today", sent[0].Body)
	assert.Equal(t, account.Integration.HookUrl, sent[0].StatusCallback)
	b := env.DB.Broadcast.GetX(ctx, id)
	assert.Equal(t, broadcast.StatusSent, b.Status)
	msg := env.DB.OutboundMessage.Query().Where(outboundmessage.BroadcastID(id)).OnlyX(ctx)
	assert.Equal(t, outboundmessage.ChannelSms, msg.Channel)
	assert.Equal(t, sent[0].SID, lo.FromPtr(msg.ProviderMessageID))
	smsSent := env.OutboxCount(t, events.NameSmsSent, map[string]any{"broadcastId": id, "contactId": fixtures.ContactTexterID})
	assert.Equal(t, 1, smsSent)
	figures := figuresOf(t, env, id)
	assert.Equal(t, broadcasts.Stats{Recipients: 1, Sent: 1}, figures)
}

// A Recipient frozen with a confirmed phone is checked again when its text is sent: one that lost
// its phone, opted out or was suppressed since is Skipped with that reason, and the figures add up.
func TestAnSMSBroadcastSkipsARecipientThatLostItsPhoneOrOptedOutWithTheReason(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := t.Context()
	account := env.ConnectTwilio(t)
	env.DB.Contact.UpdateOneID(fixtures.ContactAliceID).SetPhone(alicePhone).ExecX(ctx)
	env.DB.Contact.UpdateOneID(fixtures.ContactBobID).SetPhone(bobPhone).ExecX(ctx)
	for _, number := range []string{fixtures.ContactTexterPhone, fixtures.ContactExportSubjectPhone, alicePhone, bobPhone} {
		confirmSMS(t, env, number)
	}
	id := smsBroadcast(t, env, account, "Hello")
	mod := env.OutboundModule()

	ids, err := planAudience(ctx, env.DB, mod, id)
	require.NoError(t, err)
	env.DB.Contact.UpdateOneID(fixtures.ContactAliceID).ClearPhone().ExecX(ctx)
	_, err = env.ExternalScoped(t, "contacts:write").UnsubscribesCreate(ctx, &externalapi.CreateUnsubscribeInput{
		Channel: externalapi.NewOptConsentChannel(externalapi.ConsentChannelSMS), Destination: fixtures.ContactExportSubjectPhone,
	})
	require.NoError(t, err)
	env.SuppressNumber(ctx, bobPhone)
	for _, rid := range ids {
		require.NoError(t, deliverRecipient(ctx, env.DB, mod, rid))
	}

	assert.Len(t, ids, 4)
	sent := env.Twilio.Messages()
	texted := lo.Map(sent, func(m twiliotest.Message, _ int) string { return m.To })
	assert.Equal(t, []string{fixtures.ContactTexterPhone}, texted)
	skipped := env.DB.OutboundMessage.Query().Where(outboundmessage.BroadcastID(id), outboundmessage.StatusEQ(outboundmessage.StatusSkipped)).AllX(ctx)
	reasons := lo.Associate(skipped, func(m *ent.OutboundMessage) (int64, string) {
		return lo.FromPtr(m.ContactID), lo.FromPtr(m.Reason)
	})
	assert.Equal(t, map[int64]string{
		fixtures.ContactAliceID:         eligibility.ReasonNoDestination,
		fixtures.ContactExportSubjectID: eligibility.ReasonUnsubscribedEverything,
		fixtures.ContactBobID:           eligibility.ReasonSuppressed,
	}, reasons)
	figures := figuresOf(t, env, id)
	assert.Equal(t, broadcasts.Stats{Recipients: 4, Sent: 1, Skipped: 3}, figures)
}

func TestAnSMSBroadcastPacesAtTheProviderLimit(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := t.Context()
	account := env.ConnectTwilio(t)
	env.DB.Contact.UpdateOneID(fixtures.ContactAliceID).SetPhone(alicePhone).ExecX(ctx)
	for _, number := range []string{fixtures.ContactTexterPhone, fixtures.ContactExportSubjectPhone, alicePhone} {
		confirmSMS(t, env, number)
	}
	env.DB.Integration.UpdateOneID(int64(account.Integration.ID)).SetMaxPerSecond(4).ExecX(ctx)
	id := smsBroadcast(t, env, account, "Slowly")
	del := broadcasts.NewDelivery(env.OutboundModule())
	s := env.DB.Scoped(fixtures.AcmeID)
	start := time.Now()

	ids, err := del.Plan(ctx, s, id)
	require.NoError(t, err)
	times, err := del.Pace(ctx, s, id, ids, start)
	require.NoError(t, err)

	require.Len(t, times, 3)
	assert.Equal(t, 250*time.Millisecond, times[1].Sub(times[0]), "four a second is 250ms apart")
	assert.Equal(t, 250*time.Millisecond, times[2].Sub(times[1]))
}
