package confirmations_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent/confirmation"
	"github.com/getsphericon/sphericon/ent/event"
	"github.com/getsphericon/sphericon/internal/domain/contacts/confirmations"
	"github.com/getsphericon/sphericon/internal/domain/contacts/eligibility"
	"github.com/getsphericon/sphericon/internal/domain/orchestration/automations"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/infra/events"
	"github.com/getsphericon/sphericon/internal/kernel/channels"
	"github.com/getsphericon/sphericon/internal/kernel/linktarget"
	"github.com/getsphericon/sphericon/internal/kernel/systemevent"
	"github.com/getsphericon/sphericon/internal/testhelper"
	"github.com/getsphericon/sphericon/internal/usecase/unsubscriberecording"
)

// Consent given before the number last withdrew it changes nothing (ADR 0154): the caller is told,
// and the number stays unconfirmed.
func TestAConfirmationOlderThanTheWithdrawalIsRefused(t *testing.T) {
	env := testhelper.Setup(t)
	number := fixtures.ContactTexterPhone
	stopped := time.Date(2026, 11, 1, 10, 0, 0, 0, time.UTC)
	require.NoError(t, env.Bus.Publish(t.Context(), &events.SmsStopObserved{
		WorkspaceID: fixtures.AcmeID, Phone: number, Via: events.UnsubscribedByKeyword, DedupID: "stop", ObservedAt: stopped,
	}))

	_, err := confirmations.Confirm(t.Context(), env.Bus, env.DB.Scoped(fixtures.AcmeID), channels.SMS, number, evidence, stopped.Add(-time.Hour))

	consent, readErr := confirmations.ReadContactSMS(t.Context(), env.DB.Scoped(fixtures.AcmeID), fixtures.ContactTexterID)
	require.NoError(t, readErr)
	require.ErrorIs(t, err, confirmations.ErrSuperseded)
	assert.Nil(t, consent.Confirmation)
}

// A double opt-in link followed before the address left everything changes nothing, and is in the
// Contact's history as such.
func TestALateDoubleOptInIsRecordedAsChangingNothing(t *testing.T) {
	env := testhelper.Setup(t)
	s := env.DB.Scoped(fixtures.AcmeID)
	left := time.Date(2026, 11, 1, 10, 0, 0, 0, time.UTC)
	_, err := unsubscriberecording.New(env.Bus, env.OutboundModule(), testhelper.Module[*automations.Module](env), func() time.Time { return left }).
		Unsubscribe(t.Context(), s, fixtures.ContactAliceEmail, eligibility.SourceEverything)
	require.NoError(t, err)

	err = confirmations.RecordConfirmation(t.Context(), env.Bus, s, linktarget.Confirmation{
		WorkspaceID: fixtures.AcmeID, ContactID: fixtures.ContactAliceID, Destination: fixtures.ContactAliceEmail,
	}, "", left.Add(-time.Minute))

	require.NoError(t, err)
	history := env.DB.Event.Query().Where(
		event.ContactID(fixtures.ContactAliceID), event.Action(systemevent.ContactConsentSuperseded),
	).AllX(t.Context())
	confirmed := s.Confirmation().Query().Where(confirmation.DestinationEQ(fixtures.ContactAliceEmail)).ExistX(t.Context())
	assert.False(t, confirmed)
	require.Len(t, history, 1)
	assert.Equal(t, map[string]any{"channel": "email", "observation": "start"}, history[0].Properties)
	assert.Equal(t, left.Add(-time.Minute), history[0].OccurredAt.UTC())
}
