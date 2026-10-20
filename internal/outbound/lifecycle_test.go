package outbound

import (
	"context"
	"testing"

	"github.com/qmuntal/stateless"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent/outboundmessage"
	"github.com/getsphericon/sphericon/internal/lifecycle"
)

// row is an in-memory Outbound message; lost simulates a write that missed the claim fence.
type row struct {
	state outboundmessage.Status
	lost  bool
}

func (r *row) machine() *stateless.StateMachine {
	return newLifecycle(lifecycle.Storage[outboundmessage.Status]{
		Read: func(context.Context) (outboundmessage.Status, error) { return r.state, nil },
		Write: func(_ context.Context, _, to outboundmessage.Status) (bool, error) {
			if r.lost {
				return false, nil
			}
			r.state = to
			return true, nil
		},
	})
}

func TestPendingMovesToEachFinalStatus(t *testing.T) {
	for _, to := range []outboundmessage.Status{
		outboundmessage.StatusSent, outboundmessage.StatusSkipped, outboundmessage.StatusFailed,
	} {
		r := &row{state: outboundmessage.StatusPending}
		require.NoError(t, r.machine().FireCtx(context.Background(), to))
		assert.Equal(t, to, r.state)
	}
}

func TestAMessageCannotLeaveAFinalStatus(t *testing.T) {
	finals := []outboundmessage.Status{
		outboundmessage.StatusSent, outboundmessage.StatusSkipped, outboundmessage.StatusFailed,
	}
	for _, from := range finals {
		for _, to := range append(finals, outboundmessage.StatusPending) {
			r := &row{state: from}
			err := r.machine().FireCtx(context.Background(), to)
			require.ErrorIs(t, err, lifecycle.ErrInvalidTransition, "%s -> %s", from, to)
			assert.Equal(t, from, r.state)
		}
	}
}

func TestAWriteThatMissesTheClaimFenceDoesNotMove(t *testing.T) {
	r := &row{state: outboundmessage.StatusPending, lost: true}
	err := r.machine().FireCtx(context.Background(), outboundmessage.StatusSent)
	require.ErrorIs(t, err, lifecycle.ErrInvalidTransition)
	assert.Equal(t, outboundmessage.StatusPending, r.state)
}
