package broadcasts_test

import (
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent/broadcast"
	"github.com/getsphericon/sphericon/internal/domain/orchestration/broadcasts"
	"github.com/getsphericon/sphericon/internal/domain/sending/outboundmessages"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/infra/messaging"
)

// With no retry runtime every send's first attempt is its last: a failed recipient is given up at
// once and the run goes on to the rest, so the Broadcast settles with every recipient failed.
func TestRunInlineGivesUpAFailedRecipientAndGoesOn(t *testing.T) {
	d := newDelivery(t, time.Now)
	d.box.err = errors.New("provider down")

	require.NoError(t, d.del.RunInline(d.t.Context(), d.s, fixtures.BroadcastDraftID))

	assert.Equal(t, broadcast.StatusSent, d.env.DB.Broadcast.GetX(d.t.Context(), fixtures.BroadcastDraftID).Status)
	figures, err := broadcasts.StatsOf(d.t.Context(), d.s, fixtures.BroadcastDraftID)
	require.NoError(t, err)
	st := figures[fixtures.BroadcastDraftID]
	require.Positive(t, st.Recipients)
	assert.Equal(t, st.Recipients, st.Failed, "every recipient was tried and given up")
}

// A hold stops the run: nothing was consumed, and the recipients stay pending for a later one.
func TestRunInlineStopsAtAHold(t *testing.T) {
	d := newDelivery(t, time.Now)
	d.res.err = messaging.ErrNoProvider

	err := d.del.RunInline(d.t.Context(), d.s, fixtures.BroadcastDraftID)
	var held *broadcasts.HeldError
	require.ErrorAs(t, err, &held)
	assert.Equal(t, outboundmessages.HoldNoIntegration, held.Reason)

	figures, err := broadcasts.StatsOf(d.t.Context(), d.s, fixtures.BroadcastDraftID)
	require.NoError(t, err)
	st := figures[fixtures.BroadcastDraftID]
	require.Positive(t, st.Recipients)
	assert.Equal(t, st.Recipients, st.Pending, "no recipient was consumed")
}
