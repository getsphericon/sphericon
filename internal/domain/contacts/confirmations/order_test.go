package confirmations_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/internal/domain/contacts/confirmations"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/infra/events"
	"github.com/getsphericon/sphericon/internal/kernel/channels"
	"github.com/getsphericon/sphericon/internal/testhelper"
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
