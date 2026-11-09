package broadcasts_test

import (
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent/broadcast"
	"github.com/getsphericon/sphericon/ent/outboundmessage"
	"github.com/getsphericon/sphericon/internal/domain/orchestration/broadcasts"
	"github.com/getsphericon/sphericon/internal/domain/sending/outboundmessages"
	"github.com/getsphericon/sphericon/internal/fixtures"
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

// A Deferral stops the run: nothing was consumed, and the recipients stay pending for a later one.
// On a clock that stands still, with the Provider limit of the Workspace's default Integration
// spent before the run, so no token refills between the sends.
func TestRunInlineStopsAtADeferral(t *testing.T) {
	d := newDelivery(t, atFixedNow())
	d.env.DB.Integration.UpdateOneID(fixtures.IntegrationAcmeDefaultID).SetMaxPerSecond(1).ExecX(d.t.Context())
	for _, key := range []string{"tx:1", "tx:2", "tx:3"} {
		res, err := d.out.Send(d.t.Context(), d.s, outboundmessages.Request{
			Kind: outboundmessage.KindTransactional, Key: key, Destination: "a@example.com",
			Content: outboundmessages.EmailContent{Subject: "Receipt", Body: "<p>Hi</p>"},
		})
		require.NoError(t, err)
		require.Equal(t, outboundmessages.Sent, res.Outcome)
	}

	err := d.del.RunInline(d.t.Context(), d.s, fixtures.BroadcastDraftID)
	var deferred *broadcasts.DeferredError
	require.ErrorAs(t, err, &deferred)

	figures, err := broadcasts.StatsOf(d.t.Context(), d.s, fixtures.BroadcastDraftID)
	require.NoError(t, err)
	st := figures[fixtures.BroadcastDraftID]
	require.Positive(t, st.Recipients)
	assert.Equal(t, st.Recipients, st.Pending, "no recipient was consumed")
	assert.Len(t, d.box.sent, 3, "only the transactional sends went out")
}
