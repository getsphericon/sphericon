package broadcasts_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent/broadcast"
	"github.com/getsphericon/sphericon/internal/domain/orchestration/broadcasts"
	"github.com/getsphericon/sphericon/internal/domain/sending/outboundmessages"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// Scheduling arms marketing email, whose footer carries the Workspace's postal address (ADR 0156):
// without one the email Broadcast is refused and stays a draft with no time and no job.
func TestScheduleOfAnEmailBroadcastIsRefusedWithoutAPostalAddress(t *testing.T) {
	env := testhelper.Setup(t)
	s := env.DB.Scoped(fixtures.AcmeID)
	m := testhelper.Module[*broadcasts.Module](env)
	env.ClearPostalAddress(t, fixtures.AcmeID)
	revision := testhelper.BroadcastRevisionIn(t.Context(), t, s, fixtures.BroadcastDraftID)

	_, err := m.Schedule(t.Context(), s, fixtures.BroadcastDraftID, revision, time.Now().Add(time.Hour))

	require.ErrorIs(t, err, outboundmessages.ErrPostalAddressRequired)
	got := env.DB.Broadcast.GetX(t.Context(), fixtures.BroadcastDraftID)
	assert.Equal(t, broadcast.StatusDraft, got.Status)
	assert.Nil(t, got.ScheduledAt)
	assert.Empty(t, env.JobsOf(t, sendKind), "no send job is queued")
}

// Send arms the email Broadcast at once, so it is refused the same way and nothing is queued.
func TestSendOfAnEmailBroadcastIsRefusedWithoutAPostalAddress(t *testing.T) {
	env := testhelper.Setup(t)
	s := env.DB.Scoped(fixtures.AcmeID)
	m := testhelper.Module[*broadcasts.Module](env)
	env.ClearPostalAddress(t, fixtures.AcmeID)
	revision := testhelper.BroadcastRevisionIn(t.Context(), t, s, fixtures.BroadcastDraftID)

	_, err := m.Send(t.Context(), s, fixtures.BroadcastDraftID, revision)

	require.ErrorIs(t, err, outboundmessages.ErrPostalAddressRequired)
	status := env.DB.Broadcast.GetX(t.Context(), fixtures.BroadcastDraftID).Status
	assert.Equal(t, broadcast.StatusDraft, status)
	assert.Empty(t, env.JobsOf(t, sendKind), "no send job is queued")
}

// The address is the marketing email footer's: an SMS Broadcast carries none and is scheduled
// without it.
func TestScheduleOfAnSMSBroadcastNeedsNoPostalAddress(t *testing.T) {
	env := testhelper.Setup(t)
	env.LacunaTwilio(t)
	s := env.DB.Scoped(fixtures.LacunaID)
	m := testhelper.Module[*broadcasts.Module](env)
	env.ClearPostalAddress(t, fixtures.LacunaID)
	revision := testhelper.BroadcastRevisionIn(t.Context(), t, s, fixtures.BroadcastLacunaSmsHelloID)

	scheduled, err := m.Schedule(t.Context(), s, fixtures.BroadcastLacunaSmsHelloID, revision, time.Now().Add(time.Hour))

	require.NoError(t, err)
	assert.Equal(t, broadcast.StatusScheduled, scheduled.Status)
}
