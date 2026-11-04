package jobs_test

import (
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent/confirmation"
	"github.com/getsphericon/sphericon/ent/contact"
	"github.com/getsphericon/sphericon/ent/event"
	"github.com/getsphericon/sphericon/ent/outboundmessage"
	entsuppression "github.com/getsphericon/sphericon/ent/suppression"
	"github.com/getsphericon/sphericon/ent/unsubscribe"
	externalapi "github.com/getsphericon/sphericon/gen/external"
	"github.com/getsphericon/sphericon/internal/eligibility"
	"github.com/getsphericon/sphericon/internal/erasure"
	"github.com/getsphericon/sphericon/internal/events"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/messaging/twilio/twiliotest"
	"github.com/getsphericon/sphericon/internal/suppression"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// Erasing a Contact leaves nothing of its SMS life but its phone's hash (ADR 0021, #1032): the
// opt-out and the Suppression of the number stay, keyed by that hash, and keep refusing the number.

// textIn posts the text Twilio relays when number writes body to the account's number.
func textIn(t *testing.T, env *testhelper.TestEnv, account testhelper.TwilioAccount, sid, number, body string) {
	t.Helper()
	res := env.PostTwilioCallback(t, account.Integration.HookUrl, account.Account.AuthToken, url.Values{
		"MessageSid": {sid}, "AccountSid": {account.Account.SID}, "From": {number},
		"To": {testhelper.TwilioNumber}, "Body": {body}, "NumMedia": {"0"}, "SmsStatus": {"received"},
	})
	require.Equal(t, http.StatusOK, res.Code, res.Body.String())
}

// projectOutbox writes the Event rows of everything on the outbox, as the persist subscriber
// would (no router runs under txdb).
func projectOutbox(t *testing.T, env *testhelper.TestEnv) {
	t.Helper()
	envelopes := env.OutboxEnvelopes(t)
	for i := range envelopes {
		require.NoError(t, events.Persist(t.Context(), env.DB, envelopes[i]), envelopes[i].Name)
	}
}

// assertNothingNames checks that no Event, Outbound message, Confirmation, opt-out, Suppression or
// outbox envelope holds the number.
func assertNothingNames(t *testing.T, env *testhelper.TestEnv, number string) {
	t.Helper()
	ctx := t.Context()
	digits := strings.TrimPrefix(number, "+")
	assert.Zero(t, env.DB.Event.Query().Where(event.Phone(number)).CountX(ctx), "no Event holds the number")
	assert.Zero(t, env.DB.OutboundMessage.Query().Where(outboundmessage.Or(
		outboundmessage.Destination(number), outboundmessage.ReasonContains(digits))).CountX(ctx), "no Outbound message holds it")
	assert.Zero(t, env.DB.Confirmation.Query().Where(confirmation.Destination(number)).CountX(ctx), "no Confirmation")
	assert.Zero(t, env.DB.Unsubscribe.Query().Where(unsubscribe.Destination(number)).CountX(ctx), "no opt-out by the number")
	assert.Zero(t, env.DB.Suppression.Query().Where(entsuppression.Destination(number)).CountX(ctx), "no Suppression by the number")
	envelopes := env.OutboxEnvelopes(t)
	for i := range envelopes {
		e := &envelopes[i]
		assert.NotContainsf(t, string(e.Data)+e.DedupKey, digits, "the %s envelope names the number", e.Name)
	}
}

func TestErasingAContactWhoTextedLeavesOnlyItsPhoneHash(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := t.Context()
	s := env.DB.Scoped(fixtures.AcmeID)
	account := env.ConnectTwilio(t)
	number := fixtures.ContactTexterPhone
	automationID := smsAutomation(t, env, smsStep(account, "Hello"), smsStep(account, "See "+smsOffer))
	confirmSMS(t, env, number)
	env.Twilio.RefuseNextSend(twiliotest.Refusal{Status: http.StatusBadRequest, Code: 21211,
		Message: "The 'To' number " + number + " is not a valid phone number."})
	enrollVia(t, env, automationID, fixtures.ContactTexterID)
	runSteps(t, env)
	paths := smsShortPaths(t, env)
	require.Len(t, paths, 1)
	sent := textOf(t, env, number)
	reportStatus(t, env, account, sent, "delivered", "")
	require.Equal(t, http.StatusFound, follow(t, env, paths[0], browserUA).Code)
	textIn(t, env, account, "SM0301", number, "Thanks, my card is 4111")
	textIn(t, env, account, "SM0302", number, "STOP")
	env.SuppressNumber(ctx, number)
	projectOutbox(t, env)
	kinds := []string{events.NameSmsSent, events.NameSmsDelivered, events.NameSmsClicked}
	kept := env.DB.Event.Query().Where(event.ContactID(fixtures.ContactTexterID), event.ActionIn(kinds...)).IDsX(ctx)
	require.Len(t, kept, len(kinds))
	gone := env.DB.Event.Query().Where(event.ContactID(fixtures.ContactTexterID),
		event.ActionIn(events.NameSmsReceived, events.NameSmsConfirmed)).IDsX(ctx)
	require.Len(t, gone, 2)

	eraseContact(t, env, fixtures.ContactTexterID)

	assertNothingNames(t, env, number)
	assert.False(t, env.DB.Event.Query().Where(event.IDIn(gone...)).ExistX(ctx), "the person's words and consent proof are gone")
	for _, id := range kept {
		e := env.DB.Event.GetX(ctx, id)
		assert.Nilf(t, e.ContactID, "%s keeps no Contact", e.Action)
		assert.Nilf(t, e.Phone, "%s keeps no phone", e.Action)
		assert.NotContainsf(t, lo.FromPtr(e.SourceID), sent.SID, "%s keeps no provider reference", e.Action)
		assert.Emptyf(t, e.Properties, "%s keeps no Destination or url", e.Action)
	}
	for _, m := range env.DB.OutboundMessage.Query().Where(outboundmessage.AutomationID(automationID)).AllX(ctx) {
		assert.Nil(t, m.ContactID)
		assert.Empty(t, m.Destination)
		assert.Nil(t, m.ProviderMessageID, "no Twilio SID")
	}
	assert.Equal(t, http.StatusNotFound, follow(t, env, paths[0], browserUA).Code, "the short link answers 404")
	erased := env.OutboxEvents(t, events.NameContactErased)
	require.Len(t, erased, 1)
	hash := erased[0].(*events.ContactErased).PhoneHash
	assert.Equal(t, erasure.Hash(number), hash, "the Erasure record holds the phone's hash")
	optOut := env.DB.Unsubscribe.Query().Where(unsubscribe.ChannelEQ(unsubscribe.ChannelSms), unsubscribe.Destination(hash)).OnlyX(ctx)
	assert.Nil(t, optOut.ContactID, "the opt-out stays, by the hash, detached")
	suppressed := env.DB.Suppression.Query().Where(entsuppression.ChannelEQ(entsuppression.ChannelSms), entsuppression.Destination(hash)).OnlyX(ctx)
	assert.Nil(t, suppressed.ContactID, "the Suppression stays, by the hash, detached")
	// The Suppression is lifted, to see the opt-out refuse on its own.
	require.NoError(t, suppression.Remove(ctx, s, suppressed.ID))

	// The person is imported again with the same phone and a fresh Confirmation: no text goes.
	res, err := env.ExternalScoped(t, "contacts:write").ContactsCreate(ctx, &externalapi.CreateContactInput{
		Phone: externalapi.NewOptNilString(number),
	})
	require.NoError(t, err)
	again, ok := res.(*externalapi.ContactResource)
	require.Truef(t, ok, "create the Contact: got %T", res)
	confirmSMS(t, env, number)
	dec, err := eligibility.Check(ctx, s, eligibility.ChannelSMS, number, eligibility.SourceBroadcasts)
	require.NoError(t, err)
	assert.Equal(t, eligibility.ReasonUnsubscribedEverything, dec.Reason)
	assert.False(t, s.Contact().Query().Where(contact.ID(int64(again.ID)),
		eligibility.Audience(eligibility.PhoneDestination(), eligibility.SourceBroadcasts)).ExistX(ctx), "a Broadcast skips it")
	enrollVia(t, env, automationID, int64(again.ID))
	runSteps(t, env)
	assert.Len(t, env.Twilio.Messages(), 1, "the number is still opted out")

	// Its START lifts the opt-out the hash keeps.
	textIn(t, env, account, "SM0303", number, "START")
	dec, err = eligibility.Check(ctx, s, eligibility.ChannelSMS, number, eligibility.SourceBroadcasts)
	require.NoError(t, err)
	assert.True(t, dec.Eligible, dec.Reason)
}

// A Confirmation recorded for a number before any Contact had it names no Contact; erasing the
// Contact that later took the number removes it and its sms.confirmed proof too.
func TestErasingAContactRemovesTheConsentItsNumberGaveBeforeItWasAContact(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := t.Context()
	account := env.ConnectTwilio(t)
	const number = "+14155550177"
	confirmSMS(t, env, number)
	textIn(t, env, account, "SM0401", number, "START")
	projectOutbox(t, env)
	res, err := env.ExternalScoped(t, "contacts:write").ContactsCreate(ctx, &externalapi.CreateContactInput{
		Phone: externalapi.NewOptNilString(number),
	})
	require.NoError(t, err)
	created, ok := res.(*externalapi.ContactResource)
	require.Truef(t, ok, "create the Contact: got %T", res)
	consent := env.DB.Confirmation.Query().Where(confirmation.Destination(number)).OnlyX(ctx)
	require.Nil(t, consent.ContactID, "recorded before the Contact")
	require.Equal(t, 2, env.DB.Event.Query().Where(event.Phone(number), event.ContactIDIsNil()).CountX(ctx))

	eraseContact(t, env, int64(created.ID))

	assertNothingNames(t, env, number)
}
