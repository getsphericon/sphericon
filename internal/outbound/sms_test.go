package outbound_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent/outboundmessage"
	externalapi "github.com/getsphericon/sphericon/gen/external"
	"github.com/getsphericon/sphericon/internal/eligibility"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/outbound"
	"github.com/getsphericon/sphericon/internal/providerlimit"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// SMS through the Instance's own send path (ADR 0079): the Provider limit of the Twilio Integration
// paces its texts, and the Workspace's Send cap counts them with every other Channel's messages.

// textTo is the marketing SMS Request of key to the confirmed ContactTexter, through the account.
func textTo(t *testing.T, env *testhelper.TestEnv, account testhelper.TwilioAccount, key string) outbound.Request {
	t.Helper()
	c := env.DB.Contact.GetX(t.Context(), fixtures.ContactTexterID)
	id := int64(account.Integration.ID)
	return outbound.Request{
		Kind:          outboundmessage.KindBroadcast,
		Key:           key,
		Destination:   *c.Phone,
		Contact:       c,
		Source:        eligibility.SourceBroadcasts,
		Content:       outbound.SMSContent{Text: "Hello"},
		IntegrationID: &id,
	}
}

// connectConfirmedTexter connects a Twilio account and records the Confirmation of ContactTexter's
// phone.
func connectConfirmedTexter(t *testing.T, env *testhelper.TestEnv) testhelper.TwilioAccount {
	t.Helper()
	account := env.ConnectTwilio(t)
	_, err := env.ExternalScoped(t, "contacts:write").ConfirmationsCreate(t.Context(), &externalapi.CreateConfirmationInput{
		Channel: externalapi.ConsentChannelSMS, Destination: fixtures.ContactTexterPhone,
		Wording: "Text me offers from Acme. Reply STOP to opt out.", Source: "checkout form", Brand: "Acme",
	})
	require.NoError(t, err)
	return account
}

func sendText(t *testing.T, env *testhelper.TestEnv, req outbound.Request) outbound.Result {
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

	assert.Equal(t, outbound.Sent, first.Outcome)
	assert.Equal(t, outbound.Deferral, second.Outcome)
	assert.Equal(t, providerlimit.CauseProviderLimit, second.Cause)
	assert.Positive(t, second.Wait)
	assert.Len(t, env.Twilio.Messages(), 1, "Twilio is not asked for the deferred text")
}

func TestTheSendCapCountsTextsWithEmail(t *testing.T) {
	env := testhelper.Setup(t)
	account := connectConfirmedTexter(t, env)
	capped(t, env, fixtures.AcmeID, new(1))

	email := send(t, env.OutboundModule(), env, "bc:1")
	text := sendText(t, env, textTo(t, env, account, "sms:1"))

	require.Equal(t, outbound.Sent, email.Outcome)
	assert.Equal(t, outbound.Deferral, text.Outcome)
	assert.Equal(t, providerlimit.CauseSendCap, text.Cause)
	assert.Empty(t, env.Twilio.Messages())
}

func TestAnSMSSentCountsAgainstTheSendCapOfLaterEmail(t *testing.T) {
	env := testhelper.Setup(t)
	account := connectConfirmedTexter(t, env)
	capped(t, env, fixtures.AcmeID, new(1))

	text := sendText(t, env, textTo(t, env, account, "sms:1"))
	email := send(t, env.OutboundModule(), env, "bc:1")

	require.Equal(t, outbound.Sent, text.Outcome)
	assert.Equal(t, outbound.Deferral, email.Outcome)
	assert.Equal(t, providerlimit.CauseSendCap, email.Cause)
}
