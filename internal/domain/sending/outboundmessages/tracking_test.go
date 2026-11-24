package outboundmessages_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent/outboundmessage"
	"github.com/getsphericon/sphericon/internal/domain/contacts/engagementtracking"
	"github.com/getsphericon/sphericon/internal/domain/sending/outboundmessages"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/kernel/systemevent"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// A message whose tracking was withdrawn is stamped no more (ADR 0120): an open or a click observed
// after the withdrawal leaves its stamps as they were.
func TestAWithdrawnMessageIsNotStamped(t *testing.T) {
	env := testhelper.Setup(t)
	m := testhelper.Module[*outboundmessages.Module](env)
	s := env.DB.Scoped(fixtures.AcmeID)
	const id = fixtures.OutboundMessageTrackingWithdrawnID

	for _, action := range []string{systemevent.EmailOpened, systemevent.EmailClicked} {
		require.NoError(t, m.RecordObservation(t.Context(), s, outboundmessages.Observation{MessageID: id, Action: action, ObservedAt: time.Now()}))
	}

	msg, err := s.OutboundMessage().Get(t.Context(), id)
	require.NoError(t, err)
	assert.Nil(t, msg.OpenedAt)
	assert.Nil(t, msg.ClickedAt)
}

// Is tracked in SQL agrees with IsTracked: a message that went out tracked and was not withdrawn is
// tracked, a withdrawn one is not.
func TestTrackedAndNotTrackedSplitMessagesAsIsTrackedDoes(t *testing.T) {
	env := testhelper.Setup(t)
	s := env.DB.Scoped(fixtures.AcmeID)
	pair := []int64{fixtures.OutboundMessageTrackingSampleID, fixtures.OutboundMessageTrackingWithdrawnID}

	tracked, err := s.OutboundMessage().Query().Where(outboundmessage.IDIn(pair...), engagementtracking.TrackedMessages()).IDs(t.Context())
	require.NoError(t, err)
	untracked, err := s.OutboundMessage().Query().Where(outboundmessage.IDIn(pair...), engagementtracking.UntrackedMessages()).IDs(t.Context())
	require.NoError(t, err)
	sample, err := s.OutboundMessage().Get(t.Context(), fixtures.OutboundMessageTrackingSampleID)
	require.NoError(t, err)
	withdrawn, err := s.OutboundMessage().Get(t.Context(), fixtures.OutboundMessageTrackingWithdrawnID)
	require.NoError(t, err)

	assert.Equal(t, []int64{fixtures.OutboundMessageTrackingSampleID}, tracked)
	assert.Equal(t, []int64{fixtures.OutboundMessageTrackingWithdrawnID}, untracked)
	assert.True(t, engagementtracking.IsTracked(sample))
	assert.False(t, engagementtracking.IsTracked(withdrawn))
}
