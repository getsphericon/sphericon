package external_test

import (
	"net/http"
	"testing"

	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	externalapi "github.com/getsphericon/sphericon/gen/external"
	"github.com/getsphericon/sphericon/internal/domain/identity/apitokens"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/kernel/systemevent"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// SMS and Telegram take Engagement tracking consent as email does (ADR 0120): an SMS Destination is
// the phone number in E.164, a Telegram one the user id a Binding names.

// channelTracking is an observation of a Destination on channel through /api, occurring now.
func channelTracking(channel externalapi.EngagementTrackingChannel, destination string, state externalapi.EngagementTrackingState) *externalapi.RecordEngagementTrackingInput {
	return &externalapi.RecordEngagementTrackingInput{Channel: channel, Destination: destination, State: state}
}

// A refusal of a Contact's phone through /api is the consent of its SMS Destination, and its
// evidentiary Event is in the Contact's history; its email keeps its own.
func TestARefusalOfTrackingOfAPhoneIsTheContactsSMSConsent(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.ExternalScoped(t, apitokens.ScopeContactsWrite, apitokens.ScopeContactsRead)

	recorded, err := c.EngagementTrackingCreate(t.Context(), channelTracking(externalapi.EngagementTrackingChannelSMS, "+1 415 555 0810", externalapi.EngagementTrackingStateDenied))

	require.NoError(t, err)
	assert.Equal(t, fixtures.ContactTexterPhone, recorded.Consent.Destination, "the number in E.164")
	sms := trackingOn(t, c, fixtures.ContactTexterID, externalapi.EngagementTrackingChannelSMS)
	assert.Equal(t, fixtures.ContactTexterPhone, sms.Destination)
	assert.Equal(t, externalapi.EngagementTrackingReadingDenied, sms.State)
	assert.Equal(t, externalapi.EngagementTrackingDecisionDontTrack, sms.Effective)
	email := emailTracking(t, c, fixtures.ContactTexterID)
	assert.Equal(t, externalapi.EngagementTrackingReadingDefault, email.State, "each Destination keeps its own choice")
	denied := activity(t, env, fixtures.ContactTexterID, systemevent.SmsEngagementTrackingDenied)
	assert.Len(t, denied, 1)
}

// A refusal of a Telegram user id is the consent of the chat of the Contact Bound to it, and its
// Event is in that Contact's history.
func TestARefusalOfTrackingOfATelegramUserIsTheContactsChatConsent(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.ExternalScoped(t, apitokens.ScopeContactsWrite, apitokens.ScopeContactsRead)

	_, err := c.EngagementTrackingCreate(t.Context(), channelTracking(externalapi.EngagementTrackingChannelTelegram, fixtures.BindingLiamDestination, externalapi.EngagementTrackingStateDenied))

	require.NoError(t, err)
	chat := trackingOn(t, c, fixtures.ContactLiamID, externalapi.EngagementTrackingChannelTelegram)
	assert.Equal(t, fixtures.BindingLiamDestination, chat.Destination)
	assert.Equal(t, externalapi.EngagementTrackingReadingDenied, chat.State)
	assert.Equal(t, externalapi.EngagementTrackingProvenanceImported, chat.Provenance.Value)
	denied := activity(t, env, fixtures.ContactLiamID, systemevent.TelegramEngagementTrackingDenied)
	assert.Len(t, denied, 1)
}

// Only the person lifts a refusal, on SMS and Telegram as on email.
func TestAGrantOverARefusalOfAPhoneOrAChatIsAConflict(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.ExternalScoped(t, apitokens.ScopeContactsWrite, apitokens.ScopeContactsRead)
	destinations := map[externalapi.EngagementTrackingChannel]string{
		externalapi.EngagementTrackingChannelSMS:      fixtures.ContactTexterPhone,
		externalapi.EngagementTrackingChannelTelegram: fixtures.BindingLiamDestination,
	}
	for channel, destination := range destinations {
		_, err := c.EngagementTrackingCreate(t.Context(), channelTracking(channel, destination, externalapi.EngagementTrackingStateDenied))
		require.NoError(t, err)
	}

	_, onSMS := c.EngagementTrackingCreate(t.Context(), channelTracking(externalapi.EngagementTrackingChannelSMS, fixtures.ContactTexterPhone, externalapi.EngagementTrackingStateGranted))
	_, onTelegram := c.EngagementTrackingCreate(t.Context(), channelTracking(externalapi.EngagementTrackingChannelTelegram, fixtures.BindingLiamDestination, externalapi.EngagementTrackingStateGranted))

	testhelper.ExternalRefusal(t, onSMS, http.StatusConflict)
	testhelper.ExternalRefusal(t, onTelegram, http.StatusConflict)
	sms := trackingOn(t, c, fixtures.ContactTexterID, externalapi.EngagementTrackingChannelSMS)
	chat := trackingOn(t, c, fixtures.ContactLiamID, externalapi.EngagementTrackingChannelTelegram)
	assert.Equal(t, externalapi.EngagementTrackingReadingDenied, sms.State)
	assert.Equal(t, externalapi.EngagementTrackingReadingDenied, chat.State)
}

// A contact upsert item may carry the consent of its phone; one that carries it without a phone
// fails alone.
func TestABatchUpsertRecordsTheTrackingConsentOfItsPhone(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.ExternalScoped(t, apitokens.ScopeContactsWrite, apitokens.ScopeContactsRead)
	refusal := externalapi.NewOptContactEngagementTrackingInput(externalapi.ContactEngagementTrackingInput{
		SMS: externalapi.NewOptEngagementTrackingObservationInput(externalapi.EngagementTrackingObservationInput{State: externalapi.EngagementTrackingStateDenied}),
	})

	out, err := c.ContactsBatchUpsert(t.Context(), &externalapi.UpsertContactsInput{Contacts: []externalapi.UpsertContactInput{
		{Phone: externalapi.NewOptNilString("+14155550833"), EngagementTracking: refusal},
		{Email: externalapi.NewOptNilEmailAddress("tracking-no-phone@example.com"), EngagementTracking: refusal},
	}})

	require.NoError(t, err)
	statuses := lo.Map(out.Results, func(r externalapi.ContactBatchItemResult, _ int) externalapi.ContactBatchStatus { return r.Status })
	assert.Equal(t, []externalapi.ContactBatchStatus{externalapi.ContactBatchStatusCreated, externalapi.ContactBatchStatusFailed}, statuses)
	assert.Equal(t, "the contact has no phone", out.Results[1].Error.Value)
	read := trackingOn(t, c, int64(out.Results[0].ContactId.Value), externalapi.EngagementTrackingChannelSMS)
	assert.Equal(t, "+14155550833", read.Destination)
	assert.Equal(t, externalapi.EngagementTrackingReadingDenied, read.State)
	assert.Equal(t, externalapi.EngagementTrackingProvenanceImported, read.Provenance.Value)
}

// phoneRecordedThenRecreated records state for a new Contact of number, erases the Contact,
// creates it again and reads its SMS consent.
func phoneRecordedThenRecreated(t *testing.T, c *externalapi.Client, number string, state externalapi.EngagementTrackingState) externalapi.EngagementTrackingReading {
	t.Helper()
	first, err := c.ContactsCreate(t.Context(), &externalapi.CreateContactInput{Phone: externalapi.NewOptNilString(number)})
	require.NoError(t, err)
	_, err = c.EngagementTrackingCreate(t.Context(), channelTracking(externalapi.EngagementTrackingChannelSMS, number, state))
	require.NoError(t, err)
	require.NoError(t, c.ContactsDelete(t.Context(), externalapi.ContactsDeleteParams{ID: first.ID}))
	again, err := c.ContactsCreate(t.Context(), &externalapi.CreateContactInput{Phone: externalapi.NewOptNilString(number)})
	require.NoError(t, err)
	return trackingOn(t, c, int64(again.ID), externalapi.EngagementTrackingChannelSMS).State
}

// Erasure keeps a refusal of a number (by its hash, as it keeps an opt-out), so a Contact made
// again with the number is still not tracked, and drops a grant, so it follows the default.
func TestErasureKeepsARefusalOfTrackingOfANumberAndDropsAGrant(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.ExternalScoped(t, apitokens.ScopeContactsWrite, apitokens.ScopeContactsRead, apitokens.ScopeContactsErase)

	refused := phoneRecordedThenRecreated(t, c, "+14155550841", externalapi.EngagementTrackingStateDenied)
	granted := phoneRecordedThenRecreated(t, c, "+14155550842", externalapi.EngagementTrackingStateGranted)

	assert.Equal(t, externalapi.EngagementTrackingReadingDenied, refused)
	assert.Equal(t, externalapi.EngagementTrackingReadingDefault, granted)
}
