package unsubscriberecording_test

import (
	"strconv"
	"testing"
	"time"

	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent/confirmation"
	"github.com/getsphericon/sphericon/ent/contact"
	"github.com/getsphericon/sphericon/ent/unsubscribe"
	"github.com/getsphericon/sphericon/internal/domain/contacts/confirmations"
	"github.com/getsphericon/sphericon/internal/domain/contacts/eligibility"
	"github.com/getsphericon/sphericon/internal/domain/contacts/unsubscribes"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/infra/events"
	"github.com/getsphericon/sphericon/internal/kernel/channels"
	"github.com/getsphericon/sphericon/internal/kernel/systemevent"
	"github.com/getsphericon/sphericon/internal/testhelper"
	"github.com/getsphericon/sphericon/internal/usecase/unsubscriberecording"
)

func TestUnsubscribeValidatesTheSourceAndIsIdempotent(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := t.Context()
	s := env.DB.Scoped(fixtures.AcmeID)

	_, err := testhelper.Module[*unsubscriberecording.UseCase](env).Unsubscribe(ctx, s, "  ", eligibility.SourceBroadcasts)
	require.ErrorIs(t, err, unsubscribes.ErrDestinationEmpty)
	_, err = testhelper.Module[*unsubscriberecording.UseCase](env).Unsubscribe(ctx, s, fixtures.ContactAliceEmail, "newsletter")
	require.ErrorIs(t, err, unsubscriberecording.ErrInvalidSource)
	_, err = testhelper.Module[*unsubscriberecording.UseCase](env).Unsubscribe(ctx, s, fixtures.ContactAliceEmail, "automation:999999")
	require.ErrorIs(t, err, unsubscriberecording.ErrUnknownAutomation)

	first, err := testhelper.Module[*unsubscriberecording.UseCase](env).Unsubscribe(ctx, s, "Alice@Example.com", eligibility.SourceBroadcasts)
	require.NoError(t, err)
	assert.Equal(t, fixtures.ContactAliceEmail, first.Destination)
	assert.Equal(t, eligibility.SourceBroadcasts, first.SendingSource)

	again, err := testhelper.Module[*unsubscriberecording.UseCase](env).Unsubscribe(ctx, s, fixtures.ContactAliceEmail, eligibility.SourceBroadcasts)
	require.NoError(t, err)
	assert.Equal(t, first.ID, again.ID, "a repeat is a no-op")
}

func TestUnsubscribeAcceptsAnAutomationOfTheWorkspaceOnly(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := t.Context()
	source := "automation:" + strconv.Itoa(fixtures.AutomationWelcomeSeriesID)

	got, err := testhelper.Module[*unsubscriberecording.UseCase](env).Unsubscribe(ctx, env.DB.Scoped(fixtures.AcmeID), fixtures.ContactAliceEmail, source)
	require.NoError(t, err)
	assert.Equal(t, source, got.SendingSource)

	_, err = testhelper.Module[*unsubscriberecording.UseCase](env).Unsubscribe(ctx, env.DB.Scoped(fixtures.GlobexID), fixtures.ContactAliceEmail, source)
	assert.ErrorIs(t, err, unsubscriberecording.ErrUnknownAutomation, "another Workspace's Automation is unknown")
}

// An opt-out an import brings is recorded `via: imported` at the time it states (ADR 0138): on
// email its email.unsubscribed carries the way and the time, on sms the row does, while an opt-out
// the API records is `manual` and of now.
func TestAnImportedOptOutIsRecordedViaImportedAtItsTime(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := t.Context()
	acme := env.DB.Scoped(fixtures.AcmeID)
	optOuts := testhelper.Module[*unsubscriberecording.UseCase](env)
	left := time.Now().UTC().Add(-30 * 24 * time.Hour).Truncate(time.Second)

	_, err := optOuts.OptOut(ctx, acme, unsubscriberecording.OptOutInput{
		Channel: channels.Email, Destination: fixtures.ContactAliceEmail, Source: eligibility.SourceEverything,
		Via: events.UnsubscribedByImported, At: left,
	})
	require.NoError(t, err)
	_, err = optOuts.OptOut(ctx, acme, unsubscriberecording.OptOutInput{
		Channel: channels.SMS, Destination: fixtures.ContactTexterPhone, Via: events.UnsubscribedByImported, At: left,
	})
	require.NoError(t, err)
	_, err = optOuts.OptOut(ctx, acme, unsubscriberecording.OptOutInput{Channel: channels.Email, Destination: "by-hand@example.com"})
	require.NoError(t, err)
	unsubscribed := testhelper.Queued[*events.EmailEngagement](t, env, systemevent.EmailUnsubscribed)
	sms, err := testhelper.Module[*confirmations.Module](env).ReadContactSMS(ctx, acme, fixtures.ContactTexterID)
	require.NoError(t, err)

	vias := lo.SliceToMap(unsubscribed, func(e *events.EmailEngagement) (string, string) { return e.Email, e.Via })
	assert.Equal(t, map[string]string{
		fixtures.ContactAliceEmail: events.UnsubscribedByImported, "by-hand@example.com": events.UnsubscribedByManual,
	}, vias)
	imported, ok := lo.Find(unsubscribed, func(e *events.EmailEngagement) bool { return e.Email == fixtures.ContactAliceEmail })
	require.True(t, ok)
	assert.True(t, left.Equal(imported.ObservedAt), "the time the import states")
	require.NotNil(t, sms.OptOut)
	assert.Equal(t, unsubscribe.ViaImported, sms.OptOut.Via)
}

// The Channel states read an imported Confirmation and an imported opt-out at the times the import
// states (ADR 0138, ADR 0154), not when they were recorded, on email and, by the number's hash, on
// sms.
func TestChannelStatesReadImportedConsentAndOptOutsAtTheirStatedTimes(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := t.Context()
	acme := env.DB.Scoped(fixtures.AcmeID)
	given := time.Now().UTC().Add(-60 * 24 * time.Hour).Truncate(time.Second)
	left := given.Add(30 * 24 * time.Hour)
	evidence := confirmations.Evidence{Wording: "Send me offers", Source: "old platform", Brand: "Acme", OccurredAt: given}
	for _, consent := range []confirmations.ConfirmInput{
		{Channel: channels.Email, Destination: fixtures.ContactAliceEmail, Evidence: evidence},
		{Channel: channels.SMS, Destination: fixtures.ContactTexterPhone, Evidence: evidence},
	} {
		_, err := testhelper.Module[*confirmations.Module](env).Confirm(ctx, acme, consent)
		require.NoError(t, err)
	}
	for _, optOut := range []unsubscriberecording.OptOutInput{
		{Channel: channels.Email, Destination: fixtures.ContactAliceEmail, Source: eligibility.SourceBroadcasts},
		{Channel: channels.SMS, Destination: fixtures.ContactTexterConfirmedPhone},
	} {
		optOut.Via, optOut.At = events.UnsubscribedByImported, left
		_, err := testhelper.Module[*unsubscriberecording.UseCase](env).OptOut(ctx, acme, optOut)
		require.NoError(t, err)
	}
	ids := []int64{fixtures.ContactAliceID, fixtures.ContactTexterID, fixtures.ContactTexterConfirmedID}
	contacts := acme.Contact().Query().Where(contact.IDIn(ids...)).AllX(ctx)

	states, err := eligibility.New().ChannelStates(ctx, acme, contacts)

	require.NoError(t, err)
	imported := &eligibility.ConfirmationFact{Provenance: confirmation.ProvenanceImported, At: given}
	alice := states[fixtures.ContactAliceID].Email
	assert.Equal(t, []eligibility.OptOut{{Scope: eligibility.SourceBroadcasts, At: left}}, alice.OptOuts)
	assert.Equal(t, imported, alice.Confirmation)
	assert.Equal(t, imported, states[fixtures.ContactTexterID].SMS.Confirmation)
	stopped := states[fixtures.ContactTexterConfirmedID].SMS.OptOuts
	assert.Contains(t, stopped, eligibility.OptOut{Scope: eligibility.SourceEverything, At: left})
}
