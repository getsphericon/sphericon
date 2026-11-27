package external_test

import (
	"testing"
	"time"

	"github.com/go-faster/jx"
	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	externalapi "github.com/getsphericon/sphericon/gen/external"
	"github.com/getsphericon/sphericon/internal/domain/contacts/eligibility"
	"github.com/getsphericon/sphericon/internal/domain/identity/apitokens"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/kernel/channels"
	"github.com/getsphericon/sphericon/internal/kernel/skipreason"
	"github.com/getsphericon/sphericon/internal/kernel/systemevent"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// emailDecision is Send-eligibility of an address on email for a Broadcast in Acme.
func emailDecision(t *testing.T, env *testhelper.TestEnv, address string) eligibility.Decision {
	t.Helper()
	d, err := eligibility.New().Check(t.Context(), env.DB.Scoped(fixtures.AcmeID), eligibility.CheckInput{
		Channel: channels.Email, Destination: address, Mode: eligibility.Marketing(eligibility.SourceBroadcasts),
	})
	require.NoError(t, err)
	return d
}

// emailConsentAt is the evidence of email consent an upsert item carries, given at the time
// passed (none when it is zero).
func emailConsentAt(at time.Time) externalapi.OptImportedConsentEvidence {
	ev := externalapi.ImportedConsentEvidence{Wording: "Send me the newsletter", Source: "old platform", Brand: "Acme"}
	if !at.IsZero() {
		ev.OccurredAt = externalapi.NewOptNilTimestamp(externalapi.Timestamp(at))
	}
	return externalapi.NewOptImportedConsentEvidence(ev)
}

// upsertOne upserts the one item through c and returns its result.
func upsertOne(t *testing.T, c *externalapi.Client, item *externalapi.UpsertContactInput) externalapi.ContactBatchItemResult {
	t.Helper()
	out, err := c.ContactsBatchUpsert(t.Context(), &externalapi.UpsertContactsInput{Contacts: []externalapi.UpsertContactInput{*item}})
	require.NoError(t, err)
	require.Len(t, out.Results, 1)
	return out.Results[0]
}

// optOutOfEverything records through c that address left the Workspace's email entirely.
func optOutOfEverything(t *testing.T, c *externalapi.Client, address string) {
	t.Helper()
	_, err := c.UnsubscribesCreate(t.Context(), &externalapi.CreateUnsubscribeInput{
		Destination: address, SendingSource: externalapi.NewOptString(eligibility.SourceEverything),
	})
	require.NoError(t, err)
}

// exportOf is the data export of the Contact of address.
func exportOf(t *testing.T, env *testhelper.TestEnv, address string) externalapi.ContactExportDocument {
	t.Helper()
	res, err := env.ExternalScoped(t, apitokens.ScopeContactsExport).ContactsExport(t.Context(), externalapi.ContactsExportParams{
		Email: externalapi.NewOptEmailAddress(externalapi.EmailAddress(address)),
	})
	require.NoError(t, err)
	doc, _, _ := readExport(t, res)
	return doc
}

// exportedConfirmations are the Confirmations the data export of the Contact of address holds.
func exportedConfirmations(t *testing.T, env *testhelper.TestEnv, address string) []externalapi.ContactExportConfirmation {
	t.Helper()
	return exportOf(t, env, address).Confirmations
}

// An item's Tags are added to its Contact (ADR 0138): a Tag named for the first time is created and
// recorded as contact.tagged, one the Contact has (in any case) stays as it is, and none is removed.
func TestABatchItemAddsItsTagsCreatingTheNewOnes(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.ExternalScoped(t, apitokens.ScopeContactsWrite)

	result := upsertOne(t, c, &externalapi.UpsertContactInput{
		Email: externalapi.NewOptNilEmailAddress(fixtures.ContactAliceEmail),
		Tags:  []string{"VIP", "migrated"},
	})
	alice := exportOf(t, env, fixtures.ContactAliceEmail)
	tags := lo.Map(alice.Tags, func(tg externalapi.ContactExportTag, _ int) string { return tg.Name })

	assert.Equal(t, externalapi.ContactBatchStatusUpdated, result.Status)
	assert.ElementsMatch(t, []string{fixtures.TagVipName, fixtures.TagNewsletterName, "migrated"}, tags)
	tagged := env.QueuedCount(t, systemevent.ContactTagged, map[string]any{"contactId": fixtures.ContactAliceID})
	assert.Equal(t, 1, tagged, "only the new Tag is applied")
}

// An item's email consent records an imported email Confirmation with its evidence, which the
// Contact's data export reads back and a Workspace requiring confirmed opt-in mails (ADR 0013,
// ADR 0138).
func TestABatchItemRecordsEmailConsentAsAnImportedConfirmation(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.ExternalScoped(t, apitokens.ScopeContactsWrite)
	env.DB.Workspace.UpdateOneID(fixtures.AcmeID).SetRequireConfirmedOptIn(true).ExecX(t.Context())

	confirmed := upsertOne(t, c, &externalapi.UpsertContactInput{
		Email: externalapi.NewOptNilEmailAddress("consented@example.com"), EmailConsent: emailConsentAt(time.Time{}),
	})
	unconfirmed := upsertOne(t, c, &externalapi.UpsertContactInput{Email: externalapi.NewOptNilEmailAddress("silent@example.com")})
	noEmail := upsertOne(t, c, &externalapi.UpsertContactInput{
		SubjectId: externalapi.NewOptNilString("no-email"), EmailConsent: emailConsentAt(time.Time{}),
	})
	exported := exportedConfirmations(t, env, "consented@example.com")

	assert.Equal(t, externalapi.ContactBatchStatusCreated, confirmed.Status)
	assert.Equal(t, externalapi.ContactBatchStatusCreated, unconfirmed.Status)
	assert.Equal(t, externalapi.ContactBatchStatusFailed, noEmail.Status, "consent for an address the contact lacks")
	require.Len(t, exported, 1)
	assert.Equal(t, string(channels.Email), exported[0].Channel)
	assert.Equal(t, "imported", exported[0].Provenance)
	assert.Equal(t, "old platform", exported[0].Source.Value)
	consented := emailDecision(t, env, "consented@example.com")
	assert.True(t, consented.Eligible)
	silent := emailDecision(t, env, "silent@example.com")
	assert.Equal(t, skipreason.Unconfirmed, silent.Reason)
	proof := env.QueuedCount(t, systemevent.MarketingConfirmed, map[string]any{
		"email": "consented@example.com", "provenance": "imported", "wording": "Send me the newsletter", "brand": "Acme",
	})
	assert.Equal(t, 1, proof)
}

// Recording consent lifts no refusal (ADR 0121): an item's email consent given after the address
// left entirely records its Confirmation, and the opt-out of everything stands.
func TestEmailConsentOverAnOptOutOfEverythingLiftsNothing(t *testing.T) {
	clock := testhelper.NewClock(time.Now().UTC().Truncate(time.Second))
	env := testhelper.Setup(t, testhelper.WithClock(clock.Now))
	c := env.ExternalScoped(t, apitokens.ScopeContactsWrite)
	optOutOfEverything(t, c, "left@example.com")
	clock.Advance(time.Hour)

	result := upsertOne(t, c, &externalapi.UpsertContactInput{
		Email: externalapi.NewOptNilEmailAddress("left@example.com"), EmailConsent: emailConsentAt(time.Time{}),
	})
	exported := exportedConfirmations(t, env, "left@example.com")
	decision := emailDecision(t, env, "left@example.com")

	assert.Equal(t, externalapi.ContactBatchStatusCreated, result.Status)
	assert.Len(t, exported, 1)
	assert.Equal(t, skipreason.UnsubscribedEverything, decision.Reason)
}

// Consent an item says was given before the address left changes nothing (ADR 0154): the row is
// written, no Confirmation is recorded, and the consent joins the history as superseded.
func TestEmailConsentOlderThanTheOptOutChangesNothing(t *testing.T) {
	clock := testhelper.NewClock(time.Now().UTC().Truncate(time.Second))
	env := testhelper.Setup(t, testhelper.WithClock(clock.Now))
	c := env.ExternalScoped(t, apitokens.ScopeContactsWrite)
	upsertOne(t, c, &externalapi.UpsertContactInput{Email: externalapi.NewOptNilEmailAddress("left@example.com")})
	optOutOfEverything(t, c, "left@example.com")
	clock.Advance(time.Hour)

	result := upsertOne(t, c, &externalapi.UpsertContactInput{
		Email: externalapi.NewOptNilEmailAddress("left@example.com"), EmailConsent: emailConsentAt(clock.Now().Add(-24 * time.Hour)),
	})
	exported := exportedConfirmations(t, env, "left@example.com")

	assert.Equal(t, externalapi.ContactBatchStatusUpdated, result.Status)
	assert.Empty(t, exported)
	superseded := env.QueuedCount(t, systemevent.ContactConsentSuperseded, map[string]any{
		"email": "left@example.com", "channel": string(channels.Email), "observation": "start",
	})
	assert.Equal(t, 1, superseded)
}

// A consent time in the future is dropped and the consent recorded as given now, with a warning
// (ADR 0138): an opt-out an hour later still applies, which a consent dated a day ahead would have
// superseded.
func TestAConsentTimeInTheFutureIsStampedNowWithAWarning(t *testing.T) {
	clock := testhelper.NewClock(time.Now().UTC().Truncate(time.Second))
	env := testhelper.Setup(t, testhelper.WithClock(clock.Now))
	c := env.ExternalScoped(t, apitokens.ScopeContactsWrite)

	result := upsertOne(t, c, &externalapi.UpsertContactInput{
		Email: externalapi.NewOptNilEmailAddress("early@example.com"), EmailConsent: emailConsentAt(clock.Now().Add(24 * time.Hour)),
	})
	confirmed := exportedConfirmations(t, env, "early@example.com")
	clock.Advance(time.Hour)
	optOutOfEverything(t, c, "early@example.com")
	withdrawn := exportedConfirmations(t, env, "early@example.com")

	assert.Equal(t, externalapi.ContactBatchStatusCreated, result.Status)
	assert.Equal(t, []externalapi.ContactBatchWarning{externalapi.ContactBatchWarningConsentTimeInFuture}, result.Warnings)
	assert.Len(t, confirmed, 1)
	assert.Empty(t, withdrawn, "the opt-out an hour later withdrew it")
}

// A Custom field value its field's type does not read refuses the item, naming the field (ADR
// 0125, ADR 0138); the other items are written, and a key seen for the first time declares its
// field by use.
func TestABatchItemWithAWronglyTypedCustomFieldFailsNamingTheField(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.ExternalScoped(t, apitokens.ScopeContactsWrite)
	fields := func(key, raw string) externalapi.OptNilJsonObject {
		return externalapi.NewOptNilJsonObject(externalapi.JsonObject{key: jx.Raw(raw)})
	}

	out, err := c.ContactsBatchUpsert(t.Context(), &externalapi.UpsertContactsInput{Contacts: []externalapi.UpsertContactInput{
		{Email: externalapi.NewOptNilEmailAddress("typed-text@example.com"), CustomFields: fields("courses_completed", `"42"`)},
		{Email: externalapi.NewOptNilEmailAddress("typed-number@example.com"), CustomFields: fields("courses_completed", `42`)},
		{Email: externalapi.NewOptNilEmailAddress("typed-new@example.com"), CustomFields: fields("first_seen_key", `"anything"`)},
	}})
	require.NoError(t, err)
	statuses := lo.Map(out.Results, func(r externalapi.ContactBatchItemResult, _ int) externalapi.ContactBatchStatus { return r.Status })

	assert.Equal(t, []externalapi.ContactBatchStatus{
		externalapi.ContactBatchStatusFailed, externalapi.ContactBatchStatusCreated, externalapi.ContactBatchStatusCreated,
	}, statuses)
	assert.Contains(t, out.Results[0].Error.Or(""), "courses_completed")
}

// An imported consent's proof is dated when the consent was given (ADR 0154): an item's email
// consent and a recorded sms Confirmation that state an older time are in the Contact's history at
// that time, and one stating a time in the future at now, as its consent stamp is.
func TestAnImportedConsentsProofIsDatedWhenTheConsentWasGiven(t *testing.T) {
	clock := testhelper.NewClock(time.Now().UTC().Truncate(time.Second))
	env := testhelper.Setup(t, testhelper.WithClock(clock.Now))
	c := env.ExternalScoped(t, apitokens.ScopeContactsWrite)
	given := clock.Now().Add(-30 * 24 * time.Hour)

	upsertOne(t, c, &externalapi.UpsertContactInput{
		Email: externalapi.NewOptNilEmailAddress(fixtures.ContactAliceEmail), EmailConsent: emailConsentAt(given),
	})
	upsertOne(t, c, &externalapi.UpsertContactInput{
		Email: externalapi.NewOptNilEmailAddress(fixtures.ContactBobEmail), EmailConsent: emailConsentAt(clock.Now().Add(24 * time.Hour)),
	})
	texted := smsConsent(fixtures.ContactTexterPhone)
	texted.OccurredAt = externalapi.NewOptNilTimestamp(externalapi.Timestamp(given))
	_, err := c.ConfirmationsCreate(t.Context(), texted)
	require.NoError(t, err)
	alice := activity(t, env, fixtures.ContactAliceID, systemevent.MarketingConfirmed)
	bob := activity(t, env, fixtures.ContactBobID, systemevent.MarketingConfirmed)
	texter := activity(t, env, fixtures.ContactTexterID, systemevent.SmsConfirmed)

	require.Len(t, alice, 1)
	assert.Equal(t, given, time.Time(alice[0].OccurredAt).UTC())
	require.Len(t, bob, 1)
	assert.Equal(t, clock.Now(), time.Time(bob[0].OccurredAt).UTC(), "a time in the future is now")
	require.Len(t, texter, 1)
	assert.Equal(t, given, time.Time(texter[0].OccurredAt).UTC())
}
