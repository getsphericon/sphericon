package external_test

import (
	"net/http"
	"testing"
	"time"

	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	externalapi "github.com/getsphericon/sphericon/gen/external"
	siteapi "github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/domain/contacts/eligibility"
	"github.com/getsphericon/sphericon/internal/domain/identity/apitokens"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/kernel/systemevent"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// trackingOf is the Engagement tracking observation of an email address on /api, at the given
// time (none when zero).
func trackingOf(address string, state externalapi.EngagementTrackingState, at time.Time) *externalapi.RecordEngagementTrackingInput {
	in := &externalapi.RecordEngagementTrackingInput{
		Channel: externalapi.EngagementTrackingChannelEmail, Destination: address, State: state,
	}
	if !at.IsZero() {
		in.OccurredAt = externalapi.NewOptNilTimestamp(externalapi.Timestamp(at))
	}
	return in
}

// emailTracking is the Engagement tracking consent of a Contact's email address, read through /api.
func emailTracking(t *testing.T, c *externalapi.Client, contactID int64) externalapi.EngagementTrackingConsent {
	t.Helper()
	return trackingOn(t, c, contactID, externalapi.EngagementTrackingChannelEmail)
}

// trackingOn is the Engagement tracking consent of a Contact's one Destination on channel, read
// through /api.
func trackingOn(t *testing.T, c *externalapi.Client, contactID int64, channel externalapi.EngagementTrackingChannel) externalapi.EngagementTrackingConsent {
	t.Helper()
	read, err := c.EngagementTrackingGetForContact(t.Context(), externalapi.EngagementTrackingGetForContactParams{ContactId: externalapi.EntityId(contactID)})
	require.NoError(t, err)
	on := lo.Filter(read.Destinations, func(d externalapi.EngagementTrackingConsent, _ int) bool { return d.Channel == channel })
	require.Len(t, on, 1)
	return on[0]
}

// activity is the Events of action in a Contact's history, as a member reads it on the Contact card.
func activity(t *testing.T, env *testhelper.TestEnv, contactID int64, action string) []siteapi.SiteEventResource {
	t.Helper()
	list, err := env.SiteActor(t, fixtures.OwnerJohnEmail).SiteEventsList(t.Context(), siteapi.SiteEventsListParams{
		WorkspaceId: fixtures.AcmePublicID, ContactId: siteapi.NewOptEntityId(siteapi.EntityId(contactID)), Action: siteapi.NewOptString(action),
	})
	require.NoError(t, err)
	return list.Items
}

var (
	trackingAt   = time.Date(2026, 3, 10, 12, 0, 0, 0, time.UTC)
	trackingRead = externalapi.EngagementTrackingGetForContactParams{ContactId: externalapi.EntityId(fixtures.ContactAliceID)}
)

// A refusal recorded through /api is the Contact's consent, imported, and its evidentiary Event is
// in the Contact's history (ADR 0120).
func TestARefusalOfTrackingRecordedThroughTheAPIIsTheContactsConsent(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.ExternalScoped(t, apitokens.ScopeContactsWrite, apitokens.ScopeContactsRead)

	recorded, err := c.EngagementTrackingCreate(t.Context(), trackingOf(fixtures.ContactAliceEmail, externalapi.EngagementTrackingStateDenied, time.Time{}))

	require.NoError(t, err)
	assert.Equal(t, externalapi.EngagementTrackingOutcomeApplied, recorded.Outcome)
	read := emailTracking(t, c, fixtures.ContactAliceID)
	assert.Equal(t, fixtures.ContactAliceEmail, read.Destination)
	assert.Equal(t, externalapi.EngagementTrackingReadingDenied, read.State)
	assert.Equal(t, externalapi.EngagementTrackingProvenanceImported, read.Provenance.Value)
	assert.Equal(t, externalapi.EngagementTrackingDecisionDontTrack, read.Effective)
	denied := activity(t, env, fixtures.ContactAliceID, systemevent.EmailEngagementTrackingDenied)
	require.Len(t, denied, 1)
	assert.Equal(t, fixtures.ContactAliceEmail, denied[0].Email.Value)
}

// A Destination with nothing recorded reads as the default, which tracks until the Workspace
// changes it.
func TestADestinationWithNoConsentReadsAsTheDefault(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.ExternalScoped(t, apitokens.ScopeContactsRead)

	read := emailTracking(t, c, fixtures.ContactAliceID)

	assert.Equal(t, externalapi.EngagementTrackingReadingDefault, read.State)
	assert.False(t, read.Provenance.Set)
	assert.Equal(t, externalapi.EngagementTrackingDecisionTrack, read.Effective)
}

// Only the person lifts a refusal (ADR 0121): a grant over a standing denied is a conflict, and a
// refusal over a grant applies.
func TestAGrantOverARefusalIsAConflictAndARefusalOverAGrantApplies(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.ExternalScoped(t, apitokens.ScopeContactsWrite, apitokens.ScopeContactsRead)
	granted, err := c.EngagementTrackingCreate(t.Context(), trackingOf(fixtures.ContactAliceEmail, externalapi.EngagementTrackingStateGranted, trackingAt))
	require.NoError(t, err)

	denied, err := c.EngagementTrackingCreate(t.Context(), trackingOf(fixtures.ContactAliceEmail, externalapi.EngagementTrackingStateDenied, trackingAt.Add(time.Hour)))
	require.NoError(t, err)
	_, regrant := c.EngagementTrackingCreate(t.Context(), trackingOf(fixtures.ContactAliceEmail, externalapi.EngagementTrackingStateGranted, trackingAt.Add(2*time.Hour)))

	assert.Equal(t, externalapi.EngagementTrackingReadingGranted, granted.Consent.State)
	assert.Equal(t, externalapi.EngagementTrackingReadingDenied, denied.Consent.State)
	refused := testhelper.ExternalRefusal(t, regrant, http.StatusConflict)
	assert.Contains(t, refused.Detail.Value, "only the person can lift a refusal")
	read := emailTracking(t, c, fixtures.ContactAliceID)
	assert.Equal(t, externalapi.EngagementTrackingReadingDenied, read.State)
}

// An observation older than the one that set the consent changes nothing, and joins the Contact's
// history as superseded (ADR 0154).
func TestAnObservationOlderThanTheStandingOneChangesNothing(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.ExternalScoped(t, apitokens.ScopeContactsWrite, apitokens.ScopeContactsRead)
	_, err := c.EngagementTrackingCreate(t.Context(), trackingOf(fixtures.ContactAliceEmail, externalapi.EngagementTrackingStateGranted, trackingAt))
	require.NoError(t, err)

	late, err := c.EngagementTrackingCreate(t.Context(), trackingOf(fixtures.ContactAliceEmail, externalapi.EngagementTrackingStateDenied, trackingAt.Add(-time.Hour)))

	require.NoError(t, err)
	assert.Equal(t, externalapi.EngagementTrackingOutcomeSuperseded, late.Outcome)
	assert.Equal(t, externalapi.EngagementTrackingReadingGranted, late.Consent.State)
	read := emailTracking(t, c, fixtures.ContactAliceID)
	assert.Equal(t, externalapi.EngagementTrackingReadingGranted, read.State)
	history := activity(t, env, fixtures.ContactAliceID, systemevent.ContactConsentSuperseded)
	require.Len(t, history, 1)
	assert.Equal(t, trackingAt.Add(-time.Hour), time.Time(history[0].OccurredAt).UTC())
	assert.JSONEq(t, `"tracking_denied"`, string(history[0].Properties.Value["observation"]))
	denied := activity(t, env, fixtures.ContactAliceID, systemevent.EmailEngagementTrackingDenied)
	assert.Empty(t, denied)
}

// Of a grant and a refusal of the same moment, the refusal stands, whichever came first.
func TestAGrantAndARefusalOfTheSameMomentLeaveTheRefusal(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.ExternalScoped(t, apitokens.ScopeContactsWrite, apitokens.ScopeContactsRead)

	_, err := c.EngagementTrackingCreate(t.Context(), trackingOf(fixtures.ContactAliceEmail, externalapi.EngagementTrackingStateGranted, trackingAt))
	require.NoError(t, err)
	tie, err := c.EngagementTrackingCreate(t.Context(), trackingOf(fixtures.ContactAliceEmail, externalapi.EngagementTrackingStateDenied, trackingAt))
	require.NoError(t, err)
	_, regrant := c.EngagementTrackingCreate(t.Context(), trackingOf(fixtures.ContactAliceEmail, externalapi.EngagementTrackingStateGranted, trackingAt))

	assert.Equal(t, externalapi.EngagementTrackingOutcomeApplied, tie.Outcome)
	testhelper.ExternalRefusal(t, regrant, http.StatusConflict)
	read := emailTracking(t, c, fixtures.ContactAliceID)
	assert.Equal(t, externalapi.EngagementTrackingReadingDenied, read.State)
}

// The consent is refused for a Destination that is none of its Channel's (a number without its
// country code), for a time to come, and without contacts:write.
func TestAnObservationIsRefusedForABadDestinationInTheFutureOrWithoutContactsWrite(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.ExternalScoped(t, apitokens.ScopeContactsWrite, apitokens.ScopeContactsRead)
	onSMS := trackingOf("4155550810", externalapi.EngagementTrackingStateDenied, time.Time{})
	onSMS.Channel = externalapi.EngagementTrackingChannelSMS
	future := trackingOf(fixtures.ContactAliceEmail, externalapi.EngagementTrackingStateGranted, time.Now().Add(24*time.Hour))
	reader := env.ExternalScoped(t, apitokens.ScopeContactsRead)

	_, sms := c.EngagementTrackingCreate(t.Context(), onSMS)
	_, later := c.EngagementTrackingCreate(t.Context(), future)
	_, unscoped := reader.EngagementTrackingCreate(t.Context(), trackingOf(fixtures.ContactAliceEmail, externalapi.EngagementTrackingStateDenied, time.Time{}))

	testhelper.ExternalRefusal(t, sms, http.StatusUnprocessableEntity)
	testhelper.ExternalRefusal(t, later, http.StatusUnprocessableEntity)
	testhelper.ExternalRefusal(t, unscoped, http.StatusForbidden)
	read := emailTracking(t, c, fixtures.ContactAliceID)
	assert.Equal(t, externalapi.EngagementTrackingReadingDefault, read.State)
}

// A contact upsert item may carry the consent of its email address; one that carries it without
// an email fails alone.
func TestABatchUpsertRecordsTheTrackingConsentOfItsItems(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.ExternalScoped(t, apitokens.ScopeContactsWrite, apitokens.ScopeContactsRead)
	refusal := externalapi.NewOptContactEngagementTrackingInput(externalapi.ContactEngagementTrackingInput{
		Email: externalapi.NewOptEngagementTrackingObservationInput(externalapi.EngagementTrackingObservationInput{State: externalapi.EngagementTrackingStateDenied}),
	})

	out, err := c.ContactsBatchUpsert(t.Context(), &externalapi.UpsertContactsInput{Contacts: []externalapi.UpsertContactInput{
		{Email: externalapi.NewOptNilEmailAddress("tracking-no@example.com"), EngagementTracking: refusal},
		{Phone: externalapi.NewOptNilString("+14155550832"), EngagementTracking: refusal},
	}})

	require.NoError(t, err)
	statuses := lo.Map(out.Results, func(r externalapi.ContactBatchItemResult, _ int) externalapi.ContactBatchStatus { return r.Status })
	assert.Equal(t, []externalapi.ContactBatchStatus{externalapi.ContactBatchStatusCreated, externalapi.ContactBatchStatusFailed}, statuses)
	assert.Equal(t, "the contact has no email", out.Results[1].Error.Value)
	read := emailTracking(t, c, int64(out.Results[0].ContactId.Value))
	assert.Equal(t, externalapi.EngagementTrackingReadingDenied, read.State)
	assert.Equal(t, externalapi.EngagementTrackingProvenanceImported, read.Provenance.Value)
}

// recordedThenRecreated records state for a new Contact of address, erases the Contact, creates it
// again and reads its consent.
func recordedThenRecreated(t *testing.T, c *externalapi.Client, address string, state externalapi.EngagementTrackingState) externalapi.EngagementTrackingReading {
	t.Helper()
	first, err := c.ContactsCreate(t.Context(), &externalapi.CreateContactInput{Email: externalapi.NewOptNilEmailAddress(externalapi.EmailAddress(address))})
	require.NoError(t, err)
	_, err = c.EngagementTrackingCreate(t.Context(), trackingOf(address, state, trackingAt))
	require.NoError(t, err)
	require.NoError(t, c.ContactsDelete(t.Context(), externalapi.ContactsDeleteParams{ID: first.ID}))
	again, err := c.ContactsCreate(t.Context(), &externalapi.CreateContactInput{Email: externalapi.NewOptNilEmailAddress(externalapi.EmailAddress(address))})
	require.NoError(t, err)
	return emailTracking(t, c, int64(again.ID)).State
}

// Erasure keeps a refusal, so an address imported again is still not tracked, and drops a grant,
// so it follows the default again (ADR 0120, as an Unsubscribe and a Confirmation).
func TestErasureKeepsARefusalOfTrackingAndDropsAGrant(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.ExternalScoped(t, apitokens.ScopeContactsWrite, apitokens.ScopeContactsRead, apitokens.ScopeContactsErase)

	refused := recordedThenRecreated(t, c, "tracking-erased-no@example.com", externalapi.EngagementTrackingStateDenied)
	granted := recordedThenRecreated(t, c, "tracking-erased-yes@example.com", externalapi.EngagementTrackingStateGranted)

	assert.Equal(t, externalapi.EngagementTrackingReadingDenied, refused)
	assert.Equal(t, externalapi.EngagementTrackingReadingDefault, granted)
}

// Erasure drops the time of an erased grant, even where the address's opt-out keeps its stamp: a
// refusal of the address made again that occurred before the grant applies, since nothing of the
// grant stands.
func TestARefusalAfterErasureIsNotSupersededByTheErasedGrant(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.ExternalScoped(t, apitokens.ScopeContactsWrite, apitokens.ScopeContactsRead, apitokens.ScopeContactsErase)
	address := "tracking-erased-grant@example.com"
	first, err := c.ContactsCreate(t.Context(), &externalapi.CreateContactInput{Email: externalapi.NewOptNilEmailAddress(externalapi.EmailAddress(address))})
	require.NoError(t, err)
	_, err = c.EngagementTrackingCreate(t.Context(), trackingOf(address, externalapi.EngagementTrackingStateGranted, trackingAt))
	require.NoError(t, err)
	_, err = c.UnsubscribesCreate(t.Context(), &externalapi.CreateUnsubscribeInput{
		Destination: address, SendingSource: externalapi.NewOptString(eligibility.SourceEverything),
	})
	require.NoError(t, err)
	require.NoError(t, c.ContactsDelete(t.Context(), externalapi.ContactsDeleteParams{ID: first.ID}))

	refused, err := c.EngagementTrackingCreate(t.Context(), trackingOf(address, externalapi.EngagementTrackingStateDenied, trackingAt.Add(-time.Hour)))

	require.NoError(t, err)
	assert.Equal(t, externalapi.EngagementTrackingOutcomeApplied, refused.Outcome)
	assert.Equal(t, externalapi.EngagementTrackingReadingDenied, refused.Consent.State)
}

// Reading the consent needs contacts:read and stays in the Workspace.
func TestReadingTrackingConsentNeedsContactsReadAndStaysInTheWorkspace(t *testing.T) {
	env := testhelper.Setup(t)

	_, writer := env.ExternalScoped(t, apitokens.ScopeContactsWrite).EngagementTrackingGetForContact(t.Context(), trackingRead)
	_, foreign := env.ExternalScoped(t, apitokens.ScopeContactsRead).EngagementTrackingGetForContact(t.Context(),
		externalapi.EngagementTrackingGetForContactParams{ContactId: externalapi.EntityId(fixtures.TyrellMatrixConfirmedID)})

	testhelper.ExternalRefusal(t, writer, http.StatusForbidden)
	testhelper.ExternalRefusal(t, foreign, http.StatusNotFound)
}

// A replayed observation, the same state of the same moment, changes nothing and records nothing:
// a sync that sends the consent on every run fills no history.
func TestARepeatedObservationRecordsNothing(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.ExternalScoped(t, apitokens.ScopeContactsWrite, apitokens.ScopeContactsRead)
	_, err := c.EngagementTrackingCreate(t.Context(), trackingOf(fixtures.ContactAliceEmail, externalapi.EngagementTrackingStateGranted, trackingAt))
	require.NoError(t, err)

	again, err := c.EngagementTrackingCreate(t.Context(), trackingOf(fixtures.ContactAliceEmail, externalapi.EngagementTrackingStateGranted, trackingAt))

	require.NoError(t, err)
	assert.Equal(t, externalapi.EngagementTrackingOutcomeApplied, again.Outcome)
	history := activity(t, env, fixtures.ContactAliceID, systemevent.ContactConsentSuperseded)
	assert.Empty(t, history)
	granted := activity(t, env, fixtures.ContactAliceID, systemevent.EmailEngagementTrackingGranted)
	assert.Len(t, granted, 1)
}

// eventsOfAddress is the Events of Acme that name address, as a member finds them by it.
func eventsOfAddress(t *testing.T, env *testhelper.TestEnv, address string) []siteapi.SiteEventResource {
	t.Helper()
	list, err := env.SiteActor(t, fixtures.OwnerJohnEmail).SiteEventsList(t.Context(), siteapi.SiteEventsListParams{
		WorkspaceId: fixtures.AcmePublicID, Email: siteapi.NewOptString(address),
	})
	require.NoError(t, err)
	return list.Items
}

// The consent of an address recorded before any Contact held it is the Contact's once one does:
// its Erasure deletes the grant's Event and keeps the refusal's without the address.
func TestErasureReachesTheTrackingEventsOfAnAddressRecordedBeforeItsContact(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.ExternalScoped(t, apitokens.ScopeContactsWrite, apitokens.ScopeContactsRead, apitokens.ScopeContactsErase)
	refusing, granting := "tracking-early-no@example.com", "tracking-early-yes@example.com"
	_, err := c.EngagementTrackingCreate(t.Context(), trackingOf(refusing, externalapi.EngagementTrackingStateDenied, trackingAt))
	require.NoError(t, err)
	_, err = c.EngagementTrackingCreate(t.Context(), trackingOf(granting, externalapi.EngagementTrackingStateGranted, trackingAt))
	require.NoError(t, err)
	before := len(eventsOfAddress(t, env, refusing)) + len(eventsOfAddress(t, env, granting))
	_, err = c.ContactsCreate(t.Context(), &externalapi.CreateContactInput{Email: externalapi.NewOptNilEmailAddress(externalapi.EmailAddress(refusing))})
	require.NoError(t, err)
	_, err = c.ContactsCreate(t.Context(), &externalapi.CreateContactInput{Email: externalapi.NewOptNilEmailAddress(externalapi.EmailAddress(granting))})
	require.NoError(t, err)

	require.NoError(t, c.ContactsEraseBy(t.Context(), externalapi.ContactsEraseByParams{Email: externalapi.NewOptEmailAddress(externalapi.EmailAddress(refusing))}))
	require.NoError(t, c.ContactsEraseBy(t.Context(), externalapi.ContactsEraseByParams{Email: externalapi.NewOptEmailAddress(externalapi.EmailAddress(granting))}))

	assert.Equal(t, 2, before)
	assert.Empty(t, eventsOfAddress(t, env, refusing))
	assert.Empty(t, eventsOfAddress(t, env, granting))
}

// A refusal dated after now (a client clock ahead of ours) is not bounced: it applies as of now.
func TestARefusalDatedInTheFutureAppliesAsOfNow(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.ExternalScoped(t, apitokens.ScopeContactsWrite, apitokens.ScopeContactsRead)

	refused, err := c.EngagementTrackingCreate(t.Context(), trackingOf(fixtures.ContactAliceEmail, externalapi.EngagementTrackingStateDenied, time.Now().Add(time.Hour)))

	require.NoError(t, err)
	assert.Equal(t, externalapi.EngagementTrackingReadingDenied, refused.Consent.State)
	denied := activity(t, env, fixtures.ContactAliceID, systemevent.EmailEngagementTrackingDenied)
	require.Len(t, denied, 1)
	assert.False(t, time.Time(denied[0].OccurredAt).After(time.Now()), "dated now, not in the future")
}

// A batch item that grants over a standing refusal fails alone, with the refusal's words.
func TestABatchItemGrantingOverARefusalFails(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.ExternalScoped(t, apitokens.ScopeContactsWrite, apitokens.ScopeContactsRead)
	_, err := c.EngagementTrackingCreate(t.Context(), trackingOf(fixtures.ContactAliceEmail, externalapi.EngagementTrackingStateDenied, trackingAt))
	require.NoError(t, err)
	grant := externalapi.NewOptContactEngagementTrackingInput(externalapi.ContactEngagementTrackingInput{
		Email: externalapi.NewOptEngagementTrackingObservationInput(externalapi.EngagementTrackingObservationInput{State: externalapi.EngagementTrackingStateGranted}),
	})

	out, err := c.ContactsBatchUpsert(t.Context(), &externalapi.UpsertContactsInput{Contacts: []externalapi.UpsertContactInput{
		{Email: externalapi.NewOptNilEmailAddress(fixtures.ContactAliceEmail), EngagementTracking: grant},
	}})

	require.NoError(t, err)
	assert.Equal(t, externalapi.ContactBatchStatusFailed, out.Results[0].Status)
	assert.Contains(t, out.Results[0].Error.Value, "only the person can lift a refusal")
	read := emailTracking(t, c, fixtures.ContactAliceID)
	assert.Equal(t, externalapi.EngagementTrackingReadingDenied, read.State)
}
