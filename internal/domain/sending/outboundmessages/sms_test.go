package outboundmessages_test

import (
	"testing"

	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent/confirmation"
	"github.com/getsphericon/sphericon/ent/outboundmessage"
	"github.com/getsphericon/sphericon/ent/unsubscribe"
	externalapi "github.com/getsphericon/sphericon/gen/external"
	"github.com/getsphericon/sphericon/internal/domain/contacts/eligibility"
	"github.com/getsphericon/sphericon/internal/domain/sending/outboundmessages"
	"github.com/getsphericon/sphericon/internal/domain/sending/providerlimit"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/infra/events"
	"github.com/getsphericon/sphericon/internal/infra/messaging/twilio/twiliotest"
	"github.com/getsphericon/sphericon/internal/kernel/channels"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// SMS through the Instance's own send path (ADR 0079): the Provider limit of the Twilio Integration
// paces its texts, and the Workspace's Send cap counts them with every other Channel's messages.

// textTo is the marketing SMS Request of key to the confirmed ContactTexter, through the account.
func textTo(t *testing.T, env *testhelper.TestEnv, account testhelper.TwilioAccount, key string) outboundmessages.Request {
	t.Helper()
	c := env.DB.Contact.GetX(t.Context(), fixtures.ContactTexterID)
	id := int64(account.Integration.ID)
	return outboundmessages.Request{
		Kind:          outboundmessage.KindBroadcast,
		Key:           key,
		Destination:   *c.Phone,
		Contact:       c,
		Source:        eligibility.SourceBroadcasts,
		Content:       outboundmessages.SMSContent{Text: "Hello"},
		IntegrationID: &id,
	}
}

// connectConfirmedTexter connects a Twilio account and records the Confirmation of ContactTexter's
// phone.
func connectConfirmedTexter(t *testing.T, env *testhelper.TestEnv) testhelper.TwilioAccount {
	t.Helper()
	account := env.AcmeTwilio(t)
	_, err := env.ExternalScoped(t, "contacts:write").ConfirmationsCreate(t.Context(), &externalapi.CreateConfirmationInput{
		Channel: externalapi.ConsentChannelSMS, Destination: fixtures.ContactTexterPhone,
		Wording: "Text me offers from Acme. Reply STOP to opt out.", Source: "checkout form", Brand: "Acme",
	})
	require.NoError(t, err)
	return account
}

func sendText(t *testing.T, env *testhelper.TestEnv, req outboundmessages.Request) outboundmessages.Result {
	t.Helper()
	res, err := env.OutboundModule().Send(t.Context(), acme(env), req)
	require.NoError(t, err)
	return res
}

func TestTheProviderLimitOfATwilioIntegrationPacesItsTexts(t *testing.T) {
	env := testhelper.Setup(t)
	account := connectConfirmedTexter(t, env)
	env.DB.Integration.UpdateOneID(int64(account.Integration.ID)).SetMaxPerDay(1).ExecX(t.Context())

	first := sendText(t, env, textTo(t, env, account, "sms:1"))
	second := sendText(t, env, textTo(t, env, account, "sms:2"))

	assert.Equal(t, outboundmessages.Sent, first.Outcome)
	assert.Equal(t, outboundmessages.Deferral, second.Outcome)
	assert.Equal(t, providerlimit.CauseProviderLimit, second.Cause)
	assert.Positive(t, second.Wait)
	inbox := env.Twilio.Messages()
	assert.Len(t, inbox, 1, "Twilio is not asked for the deferred text")
}

// Twilio refuses a text with 21610 to a number that texted STOP to the sender at Twilio: that is the
// number's Channel-wide opt-out here too (ADR 0077), and the message is Skipped as the next one
// would be.
func TestATextTwilioRefusesAsOptedOutRecordsTheOptOutAndIsSkipped(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := t.Context()
	account := connectConfirmedTexter(t, env)
	env.Twilio.RefuseNextSend(twiliotest.Unsubscribed)

	res := sendText(t, env, textTo(t, env, account, "sms:1"))

	assert.Equal(t, outboundmessages.Skipped, res.Outcome)
	assert.Equal(t, eligibility.ReasonUnsubscribedEverything, res.Reason)
	msg := env.DB.OutboundMessage.GetX(ctx, res.MessageID)
	assert.Equal(t, outboundmessage.StatusSkipped, msg.Status)
	assert.Equal(t, eligibility.ReasonUnsubscribedEverything, lo.FromPtr(msg.Reason))
	stop := env.DB.Unsubscribe.Query().Where(unsubscribe.ChannelEQ(channels.SMS), unsubscribe.DestinationEQ(fixtures.ContactTexterPhone)).OnlyX(ctx)
	assert.Equal(t, eligibility.SourceEverything, stop.SendingSource)
	assert.Equal(t, unsubscribe.ViaRefusedSend, lo.FromPtr(stop.Via))
	assert.Equal(t, int64(fixtures.ContactTexterID), lo.FromPtr(stop.ContactID))
	consented := env.DB.Confirmation.Query().Where(confirmation.ChannelEQ(channels.SMS), confirmation.DestinationEQ(fixtures.ContactTexterPhone)).ExistX(ctx)
	assert.False(t, consented, "the refused number's consent ends with its stop")
	stops := env.QueuedCount(t, events.NameSmsUnsubscribed, map[string]any{
		"contactId": fixtures.ContactTexterID, "via": events.UnsubscribedByRefusedSend, "integrationId": account.Integration.ID,
	})
	assert.Equal(t, 1, stops, "the stop is recorded with sms.unsubscribed, from the text's sender")

	again := sendText(t, env, textTo(t, env, account, "sms:2"))

	assert.Equal(t, outboundmessages.Skipped, again.Outcome)
	inbox := env.Twilio.Messages()
	assert.Empty(t, inbox, "Twilio is not asked again for a number that stopped, though it would accept now")
}

func TestTheSendCapCountsTextsWithEmail(t *testing.T) {
	env := testhelper.Setup(t)
	account := connectConfirmedTexter(t, env)
	capped(t, env, fixtures.AcmeID, new(1))

	email := send(t, env.OutboundModule(), env, "bc:1")
	text := sendText(t, env, textTo(t, env, account, "sms:1"))

	require.Equal(t, outboundmessages.Sent, email.Outcome)
	assert.Equal(t, outboundmessages.Deferral, text.Outcome)
	assert.Equal(t, providerlimit.CauseSendCap, text.Cause)
	inbox := env.Twilio.Messages()
	assert.Empty(t, inbox)
}

func TestAnSMSSentCountsAgainstTheSendCapOfLaterEmail(t *testing.T) {
	env := testhelper.Setup(t)
	account := connectConfirmedTexter(t, env)
	capped(t, env, fixtures.AcmeID, new(1))

	text := sendText(t, env, textTo(t, env, account, "sms:1"))
	email := send(t, env.OutboundModule(), env, "bc:1")

	require.Equal(t, outboundmessages.Sent, text.Outcome)
	assert.Equal(t, outboundmessages.Deferral, email.Outcome)
	assert.Equal(t, providerlimit.CauseSendCap, email.Cause)
}
