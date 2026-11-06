package ingesthook_test

import (
	"net/http"
	"testing"

	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/confirmation"
	"github.com/getsphericon/sphericon/ent/outboundmessage"
	"github.com/getsphericon/sphericon/ent/unsubscribe"
	externalapi "github.com/getsphericon/sphericon/gen/external"
	"github.com/getsphericon/sphericon/internal/eligibility"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/infra/events"
	"github.com/getsphericon/sphericon/internal/kernel/channels"
	"github.com/getsphericon/sphericon/internal/outbound"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// Opting out and back in by text (ADR 0077): a STOP keyword from a number is the number's
// Channel-wide opt-out on sms, from a Contact's number or any other; START or UNSTOP lifts it and
// records a Confirmation with provenance keyword. Neither is a reply, and sphericon answers neither.

// knownNumber is the number of ContactTexter, which has neither a Confirmation nor an opt-out.
const knownNumber = fixtures.ContactTexterPhone

// confirmNumber records the Confirmation an API client asserts for the number.
func confirmNumber(t *testing.T, env *testhelper.TestEnv, number string) {
	t.Helper()
	_, err := env.ExternalScoped(t, "contacts:write").ConfirmationsCreate(t.Context(), &externalapi.CreateConfirmationInput{
		Channel: externalapi.ConsentChannelSMS, Destination: number,
		Wording: "Text me offers from Acme. Reply STOP to opt out.", Source: "checkout form", Brand: "Acme",
	})
	require.NoError(t, err)
}

// textable is Send-eligibility's decision for a marketing text to the number.
func textable(t *testing.T, env *testhelper.TestEnv, number string) eligibility.Decision {
	t.Helper()
	dec, err := eligibility.Check(t.Context(), env.DB.Scoped(fixtures.AcmeID), eligibility.ChannelSMS, number, eligibility.SourceBroadcasts)
	require.NoError(t, err)
	return dec
}

// optOutOf is the number's opt-out on sms, nil when it has none.
func optOutOf(t *testing.T, env *testhelper.TestEnv, number string) *ent.Unsubscribe {
	t.Helper()
	rows := env.DB.Unsubscribe.Query().Where(
		unsubscribe.ChannelEQ(channels.SMS), unsubscribe.DestinationEQ(number),
	).AllX(t.Context())
	require.LessOrEqual(t, len(rows), 1)
	if len(rows) == 0 {
		return nil
	}
	return rows[0]
}

// smsConfirmationOf is the number's Confirmation on sms, nil when it has none.
func smsConfirmationOf(t *testing.T, env *testhelper.TestEnv, number string) *ent.Confirmation {
	t.Helper()
	return lo.FirstOrEmpty(env.DB.Confirmation.Query().Where(
		confirmation.ChannelEQ(channels.SMS), confirmation.DestinationEQ(number),
	).AllX(t.Context()))
}

func TestASTOPFromAContactsNumberOptsTheNumberOutOfEverySMS(t *testing.T) {
	env := testhelper.Setup(t)
	sms := env.AcmeTwilio(t)
	confirmNumber(t, env, knownNumber)
	before := textable(t, env, knownNumber)
	require.True(t, before.Eligible)

	posted := env.PostTwilioCallback(t, sms.Integration.HookUrl, sms.Account.AuthToken,
		inbound(sms, "SM0101", knownNumber, " Stop\n"))

	require.Equal(t, http.StatusOK, posted.Code, posted.Body.String())
	dec := textable(t, env, knownNumber)
	assert.False(t, dec.Eligible)
	assert.Equal(t, eligibility.ReasonUnsubscribedEverything, dec.Reason)
	stop := optOutOf(t, env, knownNumber)
	require.NotNil(t, stop)
	assert.Equal(t, eligibility.SourceEverything, stop.SendingSource)
	assert.Equal(t, unsubscribe.ViaKeyword, lo.FromPtr(stop.Via))
	assert.Equal(t, int64(fixtures.ContactTexterID), lo.FromPtr(stop.ContactID))
	consent := smsConfirmationOf(t, env, knownNumber)
	assert.Nil(t, consent, "the stop drops the consent it ends")
	replies := env.QueuedCount(t, events.NameSmsReceived, nil)
	assert.Zero(t, replies, "a STOP is no reply")
}

func TestASTOPFromANumberNoContactHasStillOptsItOut(t *testing.T) {
	env := testhelper.Setup(t)
	sms := env.AcmeTwilio(t)
	contacts := env.DB.Contact.Query().CountX(t.Context())

	posted := env.PostTwilioCallback(t, sms.Integration.HookUrl, sms.Account.AuthToken,
		inbound(sms, "SM0102", "+14155550199", "стоп"))

	require.Equal(t, http.StatusOK, posted.Code, posted.Body.String())
	stop := optOutOf(t, env, "+14155550199")
	require.NotNil(t, stop)
	assert.Nil(t, stop.ContactID)
	dec := textable(t, env, "+14155550199")
	assert.Equal(t, eligibility.ReasonUnsubscribedEverything, dec.Reason)
	contactsAfter := env.DB.Contact.Query().CountX(t.Context())
	assert.Equal(t, contacts, contactsAfter, "a stranger's STOP creates no Contact")
}

func TestAStopInsideASentenceIsAReplyNotAnOptOut(t *testing.T) {
	env := testhelper.Setup(t)
	sms := env.AcmeTwilio(t)
	confirmNumber(t, env, knownNumber)

	posted := env.PostTwilioCallback(t, sms.Integration.HookUrl, sms.Account.AuthToken,
		inbound(sms, "SM0103", knownNumber, "please don't stop"))

	require.Equal(t, http.StatusOK, posted.Code, posted.Body.String())
	stop := optOutOf(t, env, knownNumber)
	assert.Nil(t, stop)
	dec := textable(t, env, knownNumber)
	assert.True(t, dec.Eligible)
	replies := env.QueuedCount(t, events.NameSmsReceived, map[string]any{"text": "please don't stop"})
	assert.Equal(t, 1, replies)
}

func TestARedeliveredSTOPRecordsOneOptOut(t *testing.T) {
	env := testhelper.Setup(t)
	sms := env.AcmeTwilio(t)

	for range 2 {
		posted := env.PostTwilioCallback(t, sms.Integration.HookUrl, sms.Account.AuthToken,
			inbound(sms, "SM0104", knownNumber, "STOP"))
		require.Equal(t, http.StatusOK, posted.Code, posted.Body.String())
	}

	stop := optOutOf(t, env, knownNumber)
	assert.NotNil(t, stop)
}

func TestSTARTAfterSTOPLiftsTheOptOutWithAKeywordConfirmationAndTheNextTextGoesOut(t *testing.T) {
	env := testhelper.Setup(t)
	sms := env.AcmeTwilio(t)
	confirmNumber(t, env, knownNumber)
	stopped := env.PostTwilioCallback(t, sms.Integration.HookUrl, sms.Account.AuthToken,
		inbound(sms, "SM0105", knownNumber, "STOP"))
	require.Equal(t, http.StatusOK, stopped.Code, stopped.Body.String())
	whileStopped := textable(t, env, knownNumber)
	require.False(t, whileStopped.Eligible)

	started := env.PostTwilioCallback(t, sms.Integration.HookUrl, sms.Account.AuthToken,
		inbound(sms, "SM0106", knownNumber, "unstop"))

	require.Equal(t, http.StatusOK, started.Code, started.Body.String())
	stop := optOutOf(t, env, knownNumber)
	assert.Nil(t, stop)
	consent := smsConfirmationOf(t, env, knownNumber)
	require.NotNil(t, consent)
	assert.Equal(t, confirmation.ProvenanceKeyword, consent.Provenance)
	assert.Equal(t, int64(fixtures.ContactTexterID), lo.FromPtr(consent.ContactID))
	smsConfirmed := env.QueuedCount(t, events.NameSmsConfirmed, map[string]any{
		"provenance": "keyword", "contactId": fixtures.ContactTexterID,
	})
	assert.Equal(t, 1, smsConfirmed)
	marketingConfirmed := env.QueuedCount(t, events.NameMarketingConfirmed, nil)
	assert.Zero(t, marketingConfirmed, "SMS consent is not email's")
	replies := env.QueuedCount(t, events.NameSmsReceived, nil)
	assert.Zero(t, replies, "a START is no reply")

	contact := env.DB.Contact.GetX(t.Context(), int64(fixtures.ContactTexterID))
	id := int64(sms.Integration.ID)
	res, err := env.OutboundModule().Send(t.Context(), env.DB.Scoped(fixtures.AcmeID), outbound.Request{
		Kind: outboundmessage.KindBroadcast, Key: "sms:after-start", Destination: knownNumber, Contact: contact,
		Source: eligibility.SourceBroadcasts, Content: outbound.SMSContent{Text: "Welcome back"}, IntegrationID: &id,
	})
	require.NoError(t, err)
	assert.Equal(t, outbound.Sent, res.Outcome)
}

func TestNeitherSTOPNorSTARTIsAnswered(t *testing.T) {
	env := testhelper.Setup(t)
	sms := env.AcmeTwilio(t)

	stopped := env.PostTwilioCallback(t, sms.Integration.HookUrl, sms.Account.AuthToken,
		inbound(sms, "SM0107", knownNumber, "STOP"))
	started := env.PostTwilioCallback(t, sms.Integration.HookUrl, sms.Account.AuthToken,
		inbound(sms, "SM0108", knownNumber, "START"))

	require.Equal(t, http.StatusOK, stopped.Code, stopped.Body.String())
	require.Equal(t, http.StatusOK, started.Code, started.Body.String())
	sent := env.Twilio.Messages()
	assert.Empty(t, sent, "no text goes back")
	assert.NotContains(t, stopped.Body.String(), "<Message")
	assert.NotContains(t, started.Body.String(), "<Message")
}

// A START from a number no Contact has lifts its stop and records its consent, keyed by the number,
// as an API client's Confirmation of a number no Contact has is: it creates no Contact.
func TestASTARTFromANumberNoContactHasLiftsItsStop(t *testing.T) {
	env := testhelper.Setup(t)
	sms := env.AcmeTwilio(t)
	contacts := env.DB.Contact.Query().CountX(t.Context())
	stopped := env.PostTwilioCallback(t, sms.Integration.HookUrl, sms.Account.AuthToken,
		inbound(sms, "SM0109", "+14155550199", "STOP"))
	require.Equal(t, http.StatusOK, stopped.Code, stopped.Body.String())

	started := env.PostTwilioCallback(t, sms.Integration.HookUrl, sms.Account.AuthToken,
		inbound(sms, "SM0110", "+14155550199", "START"))

	require.Equal(t, http.StatusOK, started.Code, started.Body.String())
	stop := optOutOf(t, env, "+14155550199")
	assert.Nil(t, stop)
	consent := smsConfirmationOf(t, env, "+14155550199")
	require.NotNil(t, consent)
	assert.Equal(t, confirmation.ProvenanceKeyword, consent.Provenance)
	assert.Nil(t, consent.ContactID)
	contactsAfter := env.DB.Contact.Query().CountX(t.Context())
	assert.Equal(t, contacts, contactsAfter)
}
