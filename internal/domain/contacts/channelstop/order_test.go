package channelstop_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/consentstamp"
	"github.com/getsphericon/sphericon/ent/event"
	"github.com/getsphericon/sphericon/internal/domain/contacts/confirmations"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/infra/events"
	"github.com/getsphericon/sphericon/internal/kernel/phone"
	"github.com/getsphericon/sphericon/internal/kernel/systemevent"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// Consent observations apply in the order they occurred, not the order they arrived (ADR 0154).

// texter is the number of ContactTexter, which has neither a Confirmation nor an opt-out.
const texter = fixtures.ContactTexterPhone

func smsConsent(t *testing.T, env *testhelper.TestEnv) confirmations.ContactSMS {
	t.Helper()
	consent, err := testhelper.Module[*confirmations.Module](env).ReadContactSMS(t.Context(), env.DB.Scoped(fixtures.AcmeID), fixtures.ContactTexterID)
	require.NoError(t, err)
	return consent
}

func TestAnSMSStartOlderThanTheStopItFollowsLeavesTheNumberOptedOut(t *testing.T) {
	env := testhelper.Setup(t)
	t1 := time.Date(2026, 11, 1, 10, 0, 0, 0, time.UTC)
	t2 := t1.Add(time.Minute)
	require.NoError(t, env.Bus.Publish(t.Context(), &events.SmsStopObserved{
		WorkspaceID: fixtures.AcmeID, Phone: texter, Via: events.UnsubscribedByKeyword, DedupID: "late-stop", ObservedAt: t2,
	}))
	require.NoError(t, env.Bus.Publish(t.Context(), &events.SmsStartObserved{
		WorkspaceID: fixtures.AcmeID, Phone: texter, DedupID: "early-start", ObservedAt: t1,
	}))
	consent := smsConsent(t, env)
	history := superseded(t, env, fixtures.ContactTexterID)
	confirmed := env.DB.Event.Query().Where(event.ContactID(fixtures.ContactTexterID), event.Action(systemevent.SmsConfirmed)).CountX(t.Context())

	require.NotNil(t, consent.OptOut, "the person's last word was stop")
	assert.Nil(t, consent.Confirmation, "the older START records no consent")
	assert.Zero(t, confirmed)
	require.Len(t, history, 1, "the START is in the Contact's history, as changing nothing")
	assert.Equal(t, t1, history[0].OccurredAt.UTC(), "at the time it occurred")
	assert.Equal(t, map[string]any{"channel": "sms", "observation": "start"}, history[0].Properties)
}

func TestAStopOlderThanTheReturnThatLiftedAnOptOutChangesNothing(t *testing.T) {
	env := testhelper.Setup(t)
	t1 := time.Date(2026, 11, 1, 10, 0, 0, 0, time.UTC)
	publish := func(ev events.DomainEvent) { t.Helper(); require.NoError(t, env.Bus.Publish(t.Context(), ev)) }
	publish(&events.SmsStopObserved{WorkspaceID: fixtures.AcmeID, Phone: texter, Via: events.UnsubscribedByKeyword, DedupID: "stop", ObservedAt: t1})
	publish(&events.SmsStartObserved{WorkspaceID: fixtures.AcmeID, Phone: texter, DedupID: "start", ObservedAt: t1.Add(2 * time.Minute)})
	publish(&events.SmsStopObserved{WorkspaceID: fixtures.AcmeID, Phone: texter, Via: events.UnsubscribedByKeyword, DedupID: "late-stop", ObservedAt: t1.Add(time.Minute)})

	consent := smsConsent(t, env)
	unsubscribed := env.QueuedCount(t, systemevent.SmsUnsubscribed, nil)
	transitions := env.QueuedCount(t, events.NameSmsOptOutRecorded, nil)
	history := superseded(t, env, fixtures.ContactTexterID)

	assert.Nil(t, consent.OptOut, "the return lifted the opt-out after the late STOP occurred")
	assert.NotNil(t, consent.Confirmation)
	assert.Equal(t, 1, unsubscribed, "the late STOP is no transition")
	assert.Equal(t, 1, transitions)
	require.Len(t, history, 1)
	assert.Equal(t, map[string]any{"channel": "sms", "observation": "stop", "via": events.UnsubscribedByKeyword}, history[0].Properties)
	assert.Equal(t, t1.Add(time.Minute), history[0].OccurredAt.UTC())
}

func TestAStopOfTheSameMomentAsAReturnStands(t *testing.T) {
	env := testhelper.Setup(t)
	t1 := time.Date(2026, 11, 1, 10, 0, 0, 0, time.UTC)
	require.NoError(t, env.Bus.Publish(t.Context(), &events.SmsStartObserved{WorkspaceID: fixtures.AcmeID, Phone: texter, DedupID: "start", ObservedAt: t1}))
	require.NoError(t, env.Bus.Publish(t.Context(), &events.SmsStopObserved{
		WorkspaceID: fixtures.AcmeID, Phone: texter, Via: events.UnsubscribedByKeyword, DedupID: "stop", ObservedAt: t1,
	}))
	consent := smsConsent(t, env)
	assert.NotNil(t, consent.OptOut, "a tie goes to the refusal")
}

func TestConcurrentObservationsOfANumberApplyInTheOrderTheyOccurred(t *testing.T) {
	env := testhelper.Setup(t, testhelper.Committed())
	t1 := time.Date(2026, 11, 1, 10, 0, 0, 0, time.UTC)
	release := env.HoldOpen(t, env.DB.Scoped(fixtures.AcmeID), func(_ *ent.Scoped, pub events.Publisher) error {
		return pub.Publish(t.Context(), &events.SmsStopObserved{
			WorkspaceID: fixtures.AcmeID, Phone: texter, Via: events.UnsubscribedByKeyword, DedupID: "stop", ObservedAt: t1.Add(time.Minute),
		})
	})
	errs := env.QueueBehind(t, release, testhelper.Writer{Name: "older start", Run: func() error {
		return env.Bus.Publish(t.Context(), &events.SmsStartObserved{
			WorkspaceID: fixtures.AcmeID, Phone: texter, DedupID: "start", ObservedAt: t1,
		})
	}})
	require.NoError(t, errs["older start"])
	consent := smsConsent(t, env)
	assert.NotNil(t, consent.OptOut, "the START waited for the STOP, then found it later")
	assert.Nil(t, consent.Confirmation)
}

func TestAStopSaidAgainKeepsAnOlderStartFromLiftingIt(t *testing.T) {
	env := testhelper.Setup(t)
	t1 := time.Date(2026, 11, 1, 10, 0, 0, 0, time.UTC)
	publish := func(ev events.DomainEvent) { t.Helper(); require.NoError(t, env.Bus.Publish(t.Context(), ev)) }
	publish(&events.SmsStopObserved{WorkspaceID: fixtures.AcmeID, Phone: texter, Via: events.UnsubscribedByKeyword, DedupID: "stop", ObservedAt: t1})
	publish(&events.SmsStopObserved{WorkspaceID: fixtures.AcmeID, Phone: texter, Via: events.UnsubscribedByKeyword, DedupID: "stop-again", ObservedAt: t1.Add(2 * time.Minute)})
	publish(&events.SmsStartObserved{WorkspaceID: fixtures.AcmeID, Phone: texter, DedupID: "start", ObservedAt: t1.Add(time.Minute)})

	consent := smsConsent(t, env)
	transitions := env.QueuedCount(t, events.NameSmsOptOutRecorded, nil)
	assert.NotNil(t, consent.OptOut, "the person stopped again after the START")
	assert.Equal(t, 1, transitions, "the second STOP is no transition")
}

// The time a number's consent was set is kept by the number's hash, never by the number: an
// Erasure has nothing of it left to find (ADR 0021).
func TestTheStampOfANumberKeepsOnlyItsHash(t *testing.T) {
	env := testhelper.Setup(t)
	require.NoError(t, env.Bus.Publish(t.Context(), &events.SmsStopObserved{
		WorkspaceID: fixtures.AcmeID, Phone: texter, Via: events.UnsubscribedByKeyword, DedupID: "stop",
	}))

	plain := env.DB.ConsentStamp.Query().Where(consentstamp.Destination(texter)).CountX(t.Context())
	hashed := env.DB.ConsentStamp.Query().Where(consentstamp.Destination(phone.Hash(texter))).CountX(t.Context())

	assert.Zero(t, plain)
	assert.Equal(t, 1, hashed)
}

func TestALaterObservationStillTransitions(t *testing.T) {
	env := testhelper.Setup(t)
	t1 := time.Date(2026, 11, 1, 10, 0, 0, 0, time.UTC)
	require.NoError(t, env.Bus.Publish(t.Context(), &events.SmsStopObserved{
		WorkspaceID: fixtures.AcmeID, Phone: texter, Via: events.UnsubscribedByKeyword, DedupID: "stop", ObservedAt: t1,
	}))
	require.NoError(t, env.Bus.Publish(t.Context(), &events.SmsStartObserved{
		WorkspaceID: fixtures.AcmeID, Phone: texter, DedupID: "start", ObservedAt: t1.Add(time.Second),
	}))
	consent := smsConsent(t, env)
	history := superseded(t, env, fixtures.ContactTexterID)
	assert.Nil(t, consent.OptOut)
	assert.NotNil(t, consent.Confirmation)
	assert.Empty(t, history)
}

// superseded are the contact.consent_superseded Events of a Contact: its observations that
// changed nothing.
func superseded(t *testing.T, env *testhelper.TestEnv, contactID int64) []*ent.Event {
	t.Helper()
	return env.DB.Event.Query().Where(
		event.ContactID(contactID), event.Action(systemevent.ContactConsentSuperseded),
	).Order(ent.Asc(event.FieldID)).AllX(t.Context())
}
