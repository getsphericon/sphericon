package callbackinterpretation_test

import (
	"fmt"
	"net/http"
	"net/url"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/kernel/systemevent"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// The core Interpreter for `sms` reads an Inbound message of the twilio Preset (ADR 0076): a reply
// from a number a Contact has is sms.received on that Contact; any other number records nothing.

// inbound is the callback Twilio posts when from texts body to the connected account's number.
func inbound(sms testhelper.TwilioAccount, sid, from, body string) url.Values {
	return url.Values{
		"MessageSid": {sid}, "SmsSid": {sid}, "AccountSid": {sms.Account.SID}, "From": {from},
		"To": {testhelper.TwilioNumber}, "Body": {body}, "NumMedia": {"0"}, "SmsStatus": {"received"},
		"ApiVersion": {"2010-04-01"},
	}
}

func TestAReplyFromAKnownNumberIsRecordedAsSmsReceivedOnItsContact(t *testing.T) {
	env := testhelper.Setup(t)
	sms := env.AcmeTwilio(t)

	posted := env.PostTwilioCallback(t, sms.Integration.HookUrl, sms.Account.AuthToken,
		inbound(sms, "SM0001", fixtures.ContactTexterPhone, "Is the sale still on?"))
	redelivered := env.PostTwilioCallback(t, sms.Integration.HookUrl, sms.Account.AuthToken,
		inbound(sms, "SM0001", fixtures.ContactTexterPhone, "Is the sale still on?"))

	require.Equal(t, http.StatusOK, posted.Code, posted.Body.String())
	require.Equal(t, http.StatusOK, redelivered.Code, redelivered.Body.String())
	got := env.QueuedMessages(t, systemevent.SmsReceived)
	require.Len(t, got, 1, "a redelivered message carries the same key, so the Event is stored and published once")
	key := fmt.Sprintf("sms:%d:SM0001", sms.Integration.ID)
	assert.Equal(t, key, got[0].Envelope.DedupKey)
	received := env.QueuedCount(t, systemevent.SmsReceived, map[string]any{
		"contactId": fixtures.ContactTexterID, "integrationId": sms.Integration.ID, "text": "Is the sale still on?",
	})
	assert.Equal(t, 1, received)
}

// A sender Twilio names in a local format is read with the country Twilio says it is in: ContactTexter's
// number, written as a US one.
func TestAReplyFromAKnownNumberInALocalFormatIsRecordedOnItsContact(t *testing.T) {
	env := testhelper.Setup(t)
	sms := env.AcmeTwilio(t)
	form := inbound(sms, "SM0004", "(415) 555-0810", "See you at 5")
	form.Set("FromCountry", "US")

	posted := env.PostTwilioCallback(t, sms.Integration.HookUrl, sms.Account.AuthToken, form)

	require.Equal(t, http.StatusOK, posted.Code, posted.Body.String())
	received := env.QueuedCount(t, systemevent.SmsReceived, map[string]any{
		"contactId": fixtures.ContactTexterID, "phone": fixtures.ContactTexterPhone, "text": "See you at 5",
	})
	assert.Equal(t, 1, received)
}

func TestATextFromAnUnknownNumberCreatesNoContactAndNoEvent(t *testing.T) {
	env := testhelper.Setup(t)
	sms := env.AcmeTwilio(t)
	contacts := env.DB.Contact.Query().CountX(t.Context())
	before := len(env.QueuedEnvelopes(t))

	stranger := env.PostTwilioCallback(t, sms.Integration.HookUrl, sms.Account.AuthToken,
		inbound(sms, "SM0002", "+14155550199", "Who is this?"))
	notANumber := env.PostTwilioCallback(t, sms.Integration.HookUrl, sms.Account.AuthToken,
		inbound(sms, "SM0003", "ACMESHOP", "Hello"))

	require.Equal(t, http.StatusOK, stranger.Code, stranger.Body.String())
	require.Equal(t, http.StatusOK, notANumber.Code, notANumber.Body.String())
	contactsAfter := env.DB.Contact.Query().CountX(t.Context())
	assert.Equal(t, contacts, contactsAfter)
	envelopes := env.QueuedEnvelopes(t)
	assert.Len(t, envelopes, before)
}
