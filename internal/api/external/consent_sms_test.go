package external_test

import (
	"context"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	externalapi "github.com/getsphericon/sphericon/gen/external"
	"github.com/getsphericon/sphericon/internal/eligibility"
	"github.com/getsphericon/sphericon/internal/events"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// smsDecision is Send-eligibility of a number on sms for a Broadcast in Acme.
func smsDecision(t *testing.T, env *testhelper.TestEnv, number string) eligibility.Decision {
	t.Helper()
	d, err := eligibility.Check(context.Background(), env.DB.Scoped(fixtures.AcmeID), eligibility.ChannelSMS, number, eligibility.SourceBroadcasts)
	require.NoError(t, err)
	return d
}

func smsConsent(destination string) *externalapi.CreateConfirmationInput {
	return &externalapi.CreateConfirmationInput{
		Channel:     externalapi.ConsentChannelSMS,
		Destination: destination,
		Wording:     "Yes, text me offers from Acme. Reply STOP to opt out.",
		Source:      "checkout form",
		Brand:       "Acme",
	}
}

// A number is not texted until an API client records the consent it collected; the evidence lands
// in sms.confirmed, never in email's marketing.confirmed, and the number becomes eligible on sms
// (ADR 0077).
func TestAnAPIAssertedSMSConfirmationRecordsItsEvidenceAndMakesTheNumberEligible(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.ExternalScoped(t, "contacts:write")
	before := smsDecision(t, env, fixtures.ContactTexterPhone)

	created, err := c.ConfirmationsCreate(t.Context(), smsConsent("+1 (415) 555-0810"))

	require.NoError(t, err)
	assert.Equal(t, eligibility.ReasonUnconfirmed, before.Reason)
	assert.Equal(t, fixtures.ContactTexterPhone, created.Destination)
	assert.Equal(t, externalapi.ConfirmationProvenanceImported, created.Provenance)
	assert.Equal(t, "Acme", created.Brand.Value)
	after := smsDecision(t, env, fixtures.ContactTexterPhone)
	assert.True(t, after.Eligible)
	evidence := env.QueuedCount(t, events.NameSmsConfirmed, map[string]any{
		"phone": fixtures.ContactTexterPhone, "contactId": fixtures.ContactTexterID, "provenance": "imported",
		"wording": "Yes, text me offers from Acme. Reply STOP to opt out.", "source": "checkout form", "brand": "Acme",
	})
	assert.Equal(t, 1, evidence)
	marketingConfirmed := env.QueuedCount(t, events.NameMarketingConfirmed, nil)
	assert.Equal(t, 0, marketingConfirmed, "marketing.confirmed is email's")
}

// A weaker provenance never replaces a stronger one: an import over a START keeps the START and
// records no second Event.
func TestAnImportedSMSConfirmationLeavesAStrongerOneStanding(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.ExternalScoped(t, "contacts:write")

	standing, err := c.ConfirmationsCreate(t.Context(), smsConsent(fixtures.ConfirmationSmsKeywordDestination))

	require.NoError(t, err)
	assert.Equal(t, externalapi.ConfirmationProvenanceKeyword, standing.Provenance)
	assert.False(t, standing.Wording.Set, "the START carries no wording")
	smsConfirmed := env.QueuedCount(t, events.NameSmsConfirmed, nil)
	assert.Equal(t, 0, smsConfirmed)
}

// A stronger provenance replaces a weaker one with its own evidence.
func TestAnImportedSMSConfirmationReplacesAGrandfatheredOne(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.ExternalScoped(t, "contacts:write")

	standing, err := c.ConfirmationsCreate(t.Context(), smsConsent(fixtures.ConfirmationSmsGrandfatheredDestination))

	require.NoError(t, err)
	assert.Equal(t, externalapi.ConfirmationProvenanceImported, standing.Provenance)
	assert.Equal(t, "checkout form", standing.Source.Value)
	smsConfirmed := env.QueuedCount(t, events.NameSmsConfirmed, map[string]any{"phone": fixtures.ConfirmationSmsGrandfatheredDestination})
	assert.Equal(t, 1, smsConfirmed)
}

func TestAnSMSConfirmationIsRefusedOffSMSWithoutACountryCodeOrWithoutEvidence(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.ExternalScoped(t, "contacts:write")
	onEmail := smsConsent("texter@example.com")
	onEmail.Channel = externalapi.ConsentChannelEmail
	blankBrand := smsConsent(fixtures.ContactTexterPhone)
	blankBrand.Brand = "  "

	_, email := c.ConfirmationsCreate(t.Context(), onEmail)
	_, local := c.ConfirmationsCreate(t.Context(), smsConsent("(415) 555-0810"))
	_, brand := c.ConfirmationsCreate(t.Context(), blankBrand)

	testhelper.ExternalRefusal(t, email, http.StatusUnprocessableEntity)
	testhelper.ExternalRefusal(t, local, http.StatusUnprocessableEntity)
	testhelper.ExternalRefusal(t, brand, http.StatusUnprocessableEntity)
	decision := smsDecision(t, env, fixtures.ContactTexterPhone)
	assert.Equal(t, eligibility.ReasonUnconfirmed, decision.Reason)
}

// An SMS opt-out that reached the Workspace by another route blocks the number on every sender,
// even with a Confirmation, and recording consent afterwards does not lift it.
func TestAnOffChannelSMSOptOutBlocksTheNumber(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.ExternalScoped(t, "contacts:write")

	res, err := c.UnsubscribesCreate(t.Context(), &externalapi.CreateUnsubscribeInput{
		Channel:     externalapi.NewOptConsentChannel(externalapi.ConsentChannelSMS),
		Destination: "+1 415 555 0810",
	})
	require.NoError(t, err)
	_, err = c.ConfirmationsCreate(t.Context(), smsConsent(fixtures.ContactTexterPhone))
	require.NoError(t, err)

	created := res
	assert.Equal(t, externalapi.ConsentChannelSMS, created.Channel)
	assert.Equal(t, fixtures.ContactTexterPhone, created.Destination)
	assert.Equal(t, eligibility.SourceEverything, created.SendingSource)
	decision := smsDecision(t, env, fixtures.ContactTexterPhone)
	assert.Equal(t, eligibility.ReasonUnsubscribedEverything, decision.Reason)
}

// An opt-out of everything on sms drops the number's Confirmation, however strong: stale consent
// never reactivates, so an import after it stands instead of the START it would not replace.
func TestAnSMSOptOutDropsTheNumbersConfirmation(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.ExternalScoped(t, "contacts:write")

	_, err := c.UnsubscribesCreate(t.Context(), &externalapi.CreateUnsubscribeInput{
		Channel:     externalapi.NewOptConsentChannel(externalapi.ConsentChannelSMS),
		Destination: fixtures.ConfirmationSmsKeywordDestination,
	})
	require.NoError(t, err)
	res, err := c.ConfirmationsCreate(t.Context(), smsConsent(fixtures.ConfirmationSmsKeywordDestination))
	require.NoError(t, err)

	standing := res
	assert.Equal(t, externalapi.ConfirmationProvenanceImported, standing.Provenance)
}

func TestAnSMSOptOutIsForEverything(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.ExternalScoped(t, "contacts:write")

	_, err := c.UnsubscribesCreate(t.Context(), &externalapi.CreateUnsubscribeInput{
		Channel:       externalapi.NewOptConsentChannel(externalapi.ConsentChannelSMS),
		Destination:   fixtures.ContactTexterPhone,
		SendingSource: externalapi.NewOptString(eligibility.SourceBroadcasts),
	})

	testhelper.ExternalRefusal(t, err, http.StatusUnprocessableEntity)
}

// The import (the batch upsert) records an imported Confirmation from an item's consent columns;
// an item without them leaves its number unreachable on sms, and consent without a phone fails
// that item alone.
func TestABatchUpsertRecordsSMSConsentFromItsItems(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.ExternalScoped(t, "contacts:write")
	evidence := externalapi.NewOptConsentEvidence(externalapi.ConsentEvidence{
		Wording: "Text me offers", Source: "store signup sheet", Brand: "Acme",
	})

	out, err := c.ContactsBatchUpsert(t.Context(), &externalapi.UpsertContactsInput{Contacts: []externalapi.UpsertContactInput{
		{Email: externalapi.NewOptNilEmailAddress("sms-yes@example.com"), Phone: externalapi.NewOptNilString("+14155550830"), SmsConsent: evidence},
		{Email: externalapi.NewOptNilEmailAddress("sms-none@example.com"), Phone: externalapi.NewOptNilString("+14155550831")},
		{Email: externalapi.NewOptNilEmailAddress("sms-nophone@example.com"), SmsConsent: evidence},
	}})

	require.NoError(t, err)
	statuses := []externalapi.ContactBatchStatus{out.Results[0].Status, out.Results[1].Status, out.Results[2].Status}
	assert.Equal(t, []externalapi.ContactBatchStatus{
		externalapi.ContactBatchStatusCreated, externalapi.ContactBatchStatusCreated, externalapi.ContactBatchStatusFailed,
	}, statuses)
	withConsent := smsDecision(t, env, "+14155550830")
	assert.True(t, withConsent.Eligible)
	withoutConsent := smsDecision(t, env, "+14155550831")
	assert.Equal(t, eligibility.ReasonUnconfirmed, withoutConsent.Reason)
	smsConfirmed := env.QueuedCount(t, events.NameSmsConfirmed, map[string]any{"phone": "+14155550830", "source": "store signup sheet"})
	assert.Equal(t, 1, smsConfirmed)
	marketingConfirmed := env.QueuedCount(t, events.NameMarketingConfirmed, nil)
	assert.Equal(t, 0, marketingConfirmed, "marketing.confirmed is email's")
}
