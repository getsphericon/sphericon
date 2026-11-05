package jobs_test

import (
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/go-faster/jx"
	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/confirmation"
	"github.com/getsphericon/sphericon/ent/contact"
	"github.com/getsphericon/sphericon/ent/event"
	"github.com/getsphericon/sphericon/ent/outboundmessage"
	entsuppression "github.com/getsphericon/sphericon/ent/suppression"
	"github.com/getsphericon/sphericon/ent/unsubscribe"
	externalapi "github.com/getsphericon/sphericon/gen/external"
	"github.com/getsphericon/sphericon/internal/channels"
	"github.com/getsphericon/sphericon/internal/eligibility"
	"github.com/getsphericon/sphericon/internal/erasure"
	"github.com/getsphericon/sphericon/internal/events"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/messaging/twilio/twiliotest"
	"github.com/getsphericon/sphericon/internal/phone"
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

// assertNothingNames checks that no Event, Outbound message, Confirmation, opt-out, Suppression or
// queued event job holds the number.
func assertNothingNames(t *testing.T, env *testhelper.TestEnv, number string) {
	t.Helper()
	ctx := t.Context()
	digits := strings.TrimPrefix(number, "+")
	evs := env.DB.Event.Query().Where(event.Phone(number)).CountX(ctx)
	assert.Zero(t, evs, "no Event holds the number")
	msgs := env.DB.OutboundMessage.Query().Where(outboundmessage.Or(
		outboundmessage.Destination(number), outboundmessage.ReasonContains(digits))).CountX(ctx)
	assert.Zero(t, msgs, "no Outbound message holds it")
	consents := env.DB.Confirmation.Query().Where(confirmation.Destination(number)).CountX(ctx)
	assert.Zero(t, consents, "no Confirmation")
	stops := env.DB.Unsubscribe.Query().Where(unsubscribe.Destination(number)).CountX(ctx)
	assert.Zero(t, stops, "no opt-out by the number")
	suppressions := env.DB.Suppression.Query().Where(entsuppression.Destination(number)).CountX(ctx)
	assert.Zero(t, suppressions, "no Suppression by the number")
	envelopes := env.QueuedEnvelopes(t)
	naming := lo.FilterMap(envelopes, func(e events.Envelope, _ int) (string, bool) {
		return e.Name, strings.Contains(string(e.Data)+e.DedupKey, digits)
	})
	assert.Empty(t, naming, "no queued event names the number")
}

// erasedEvent is what an Event that outlives the Erasure keeps of the person: nothing.
type erasedEvent struct {
	Action     string
	ContactID  *int64
	Phone      *string
	NamesSID   bool
	Properties int
}

// erasedMessage is what an Outbound message keeps of the person after the Erasure: nothing.
type erasedMessage struct {
	ContactID         *int64
	Destination       string
	ProviderMessageID *string
}

func TestErasingAContactWhoTextedLeavesOnlyItsPhoneHash(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := t.Context()
	s := env.DB.Scoped(fixtures.AcmeID)
	account := env.AcmeTwilio(t)
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
	clicked := follow(t, env, paths[0], browserUA)
	require.Equal(t, http.StatusFound, clicked.Code)
	textIn(t, env, account, "SM0301", number, "Thanks, my card is 4111")
	textIn(t, env, account, "SM0302", number, "STOP")
	env.SuppressNumber(ctx, number)
	kinds := []string{events.NameSmsClicked, events.NameSmsDelivered, events.NameSmsSent}
	kept := env.DB.Event.Query().Where(event.ContactID(fixtures.ContactTexterID), event.ActionIn(kinds...)).IDsX(ctx)
	require.Len(t, kept, len(kinds))
	gone := env.DB.Event.Query().Where(event.ContactID(fixtures.ContactTexterID),
		event.ActionIn(events.NameSmsReceived, events.NameSmsConfirmed)).IDsX(ctx)
	require.Len(t, gone, 2)

	eraseContact(t, env, fixtures.ContactTexterID)

	assertNothingNames(t, env, number)
	stillThere := env.DB.Event.Query().Where(event.IDIn(gone...)).ExistX(ctx)
	assert.False(t, stillThere, "the person's words and consent proof are gone")
	outlived := env.DB.Event.Query().Where(event.IDIn(kept...)).Order(event.ByAction()).AllX(ctx)
	keeps := lo.Map(outlived, func(e *ent.Event, _ int) erasedEvent {
		return erasedEvent{Action: e.Action, ContactID: e.ContactID, Phone: e.Phone,
			NamesSID: strings.Contains(lo.FromPtr(e.SourceID), sent.SID), Properties: len(e.Properties)}
	})
	assert.Equal(t, []erasedEvent{
		{Action: events.NameSmsClicked}, {Action: events.NameSmsDelivered}, {Action: events.NameSmsSent},
	}, keeps, "the facts keep no Contact, phone, provider reference, Destination or url")
	texts := env.DB.OutboundMessage.Query().Where(outboundmessage.AutomationID(automationID)).AllX(ctx)
	anonymous := lo.Map(texts, func(m *ent.OutboundMessage, _ int) erasedMessage {
		return erasedMessage{ContactID: m.ContactID, Destination: m.Destination, ProviderMessageID: m.ProviderMessageID}
	})
	assert.Equal(t, []erasedMessage{{}, {}}, anonymous, "the texts keep no Contact, number or Twilio SID")
	followed := follow(t, env, paths[0], browserUA)
	assert.Equal(t, http.StatusNotFound, followed.Code, "the short link answers 404")
	erased := env.QueuedEvents(t, events.NameContactErased)
	require.Len(t, erased, 1)
	hash := erased[0].(*events.ContactErased).PhoneHash
	assert.Equal(t, erasure.Hash(number), hash, "the Erasure record holds the phone's hash")
	optOut := env.DB.Unsubscribe.Query().Where(unsubscribe.ChannelEQ(channels.SMS), unsubscribe.Destination(hash)).OnlyX(ctx)
	assert.Nil(t, optOut.ContactID, "the opt-out stays, by the hash, detached")
	suppressed := env.DB.Suppression.Query().Where(entsuppression.ChannelEQ(channels.SMS), entsuppression.Destination(hash)).OnlyX(ctx)
	assert.Nil(t, suppressed.ContactID, "the Suppression stays, by the hash, detached")
	// The Suppression is lifted, to see the opt-out refuse on its own.
	require.NoError(t, suppression.Remove(ctx, s, suppressed.ID))

	// The person is imported again with the same phone and a fresh Confirmation: no text goes.
	again, err := env.ExternalScoped(t, "contacts:write").ContactsCreate(ctx, &externalapi.CreateContactInput{
		Phone: externalapi.NewOptNilString(number),
	})
	require.NoError(t, err)
	confirmSMS(t, env, number)
	dec, err := eligibility.Check(ctx, s, eligibility.ChannelSMS, number, eligibility.SourceBroadcasts)
	require.NoError(t, err)
	assert.Equal(t, eligibility.ReasonUnsubscribedEverything, dec.Reason)
	reached := s.Contact().Query().Where(contact.ID(int64(again.ID)),
		eligibility.Audience(eligibility.PhoneDestination(), eligibility.SourceBroadcasts)).ExistX(ctx)
	assert.False(t, reached, "a Broadcast skips it")
	enrollVia(t, env, automationID, int64(again.ID))
	runSteps(t, env)
	inbox := env.Twilio.Messages()
	assert.Len(t, inbox, 1, "the number is still opted out")

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
	account := env.AcmeTwilio(t)
	const number = "+14155550177"
	confirmSMS(t, env, number)
	textIn(t, env, account, "SM0401", number, "START")
	created, err := env.ExternalScoped(t, "contacts:write").ContactsCreate(ctx, &externalapi.CreateContactInput{
		Phone: externalapi.NewOptNilString(number),
	})
	require.NoError(t, err)
	consent := env.DB.Confirmation.Query().Where(confirmation.Destination(number)).OnlyX(ctx)
	require.Nil(t, consent.ContactID, "recorded before the Contact")
	contactless := env.DB.Event.Query().Where(event.Phone(number), event.ContactIDIsNil()).CountX(ctx)
	require.Equal(t, 2, contactless)

	eraseContact(t, env, int64(created.ID))

	assertNothingNames(t, env, number)
}

// reimport creates a Contact with number again, as an import after an Erasure does, with a fresh
// SMS Confirmation, and returns its id.
func reimport(t *testing.T, env *testhelper.TestEnv, number string) int64 {
	t.Helper()
	created, err := env.ExternalScoped(t, "contacts:write").ContactsCreate(t.Context(), &externalapi.CreateContactInput{
		Phone: externalapi.NewOptNilString(number),
	})
	require.NoError(t, err)
	confirmSMS(t, env, number)
	return int64(created.ID)
}

// A number suppressed again after its Contact's Erasure has two Suppressions: the one by its hash
// the Erasure left and the new one. Lifting the new one lifts the hashed one too, so the number
// can be texted again.
func TestLiftingTheSuppressionOfANumberThatCameBackLiftsItsErasedTwin(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := t.Context()
	s := env.DB.Scoped(fixtures.AcmeID)
	number := fixtures.ContactTexterPhone
	env.SuppressNumber(ctx, number)
	eraseContact(t, env, fixtures.ContactTexterID)
	reimport(t, env, number)
	again, err := suppression.AddManual(ctx, s, channels.SMS, number)
	require.NoError(t, err)
	before, err := eligibility.Check(ctx, s, eligibility.ChannelSMS, number, eligibility.SourceBroadcasts)
	require.NoError(t, err)

	require.NoError(t, suppression.Remove(ctx, s, again.ID))

	after, err := eligibility.Check(ctx, s, eligibility.ChannelSMS, number, eligibility.SourceBroadcasts)
	require.NoError(t, err)
	assert.Equal(t, eligibility.ReasonSuppressed, before.Reason)
	assert.True(t, after.Eligible, after.Reason)
	standing := s.Suppression().Query().Where(entsuppression.ChannelEQ(channels.SMS),
		entsuppression.DestinationIn(number, phone.Hash(number)), entsuppression.LiftedAtIsNil()).CountX(ctx)
	assert.Zero(t, standing, "neither the number's nor its hash's Suppression stands")
}

// The export of a Contact that took back an erased person's number lists the refusals the hash
// keeps, since they refuse its texts too.
func TestTheExportOfAContactWithAnErasedNumberListsTheRefusalsItsHashKeeps(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := t.Context()
	number := fixtures.ContactTexterPhone
	_, err := env.ExternalScoped(t, "contacts:write").UnsubscribesCreate(ctx, &externalapi.CreateUnsubscribeInput{
		Channel: externalapi.NewOptConsentChannel(externalapi.ConsentChannelSMS), Destination: number,
	})
	require.NoError(t, err)
	env.SuppressNumber(ctx, number)
	eraseContact(t, env, fixtures.ContactTexterID)
	id := reimport(t, env, number)

	res, err := env.ExternalScoped(t, "contacts:export").ContactsExport(ctx, externalapi.ContactsExportParams{ID: externalapi.NewOptEntityId(externalapi.EntityId(id))})
	require.NoError(t, err)

	exported, ok := res.(*externalapi.ContactsExportOKApplicationOctetStreamHeaders)
	require.Truef(t, ok, "got %T", res)
	raw, err := io.ReadAll(exported.Response)
	require.NoError(t, err)
	var doc externalapi.ContactExportDocument
	require.NoError(t, doc.Decode(jx.DecodeBytes(raw)), string(raw))
	stops := lo.Map(doc.Unsubscribes, func(u externalapi.ContactExportUnsubscribe, _ int) string { return u.Channel + ":" + u.Destination })
	suppressions := lo.Map(doc.Suppressions, func(u externalapi.ContactExportSuppression, _ int) string { return u.Channel + ":" + u.Destination })
	assert.Equal(t, []string{"sms:" + phone.Hash(number)}, stops)
	assert.Equal(t, []string{"sms:" + phone.Hash(number)}, suppressions)
}
