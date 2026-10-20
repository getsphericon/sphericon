package lifecycle_test

import (
	"context"
	"errors"
	"testing"

	"github.com/qmuntal/stateless"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/internal/lifecycle"
)

type status string

type trigger string

const (
	draft  status = "draft"
	sent   status = "sent"
	failed status = "failed"

	send trigger = "send"
)

// row is an in-memory entity. Its write is the conditional update: it stores the new state
// only when it reports a swap. lost simulates a concurrent writer that moved the row after the
// machine read it (a compare-and-swap miss); fault is a storage error.
type row struct {
	state status
	lost  bool
	fault error
}

func (r *row) storage() lifecycle.Storage[status] {
	return lifecycle.Storage[status]{
		Read: func(context.Context) (status, error) { return r.state, nil },
		Write: func(_ context.Context, _, to status) (bool, error) {
			if r.fault != nil {
				return false, r.fault
			}
			if r.lost {
				return false, nil
			}
			r.state = to
			return true, nil
		},
	}
}

func machine(r *row) *stateless.StateMachine {
	m := lifecycle.New(r.storage())
	m.Configure(draft).Permit(send, sent)
	return m
}

func TestLegalMoveWritesTheNewState(t *testing.T) {
	r := &row{state: draft}
	err := machine(r).FireCtx(context.Background(), send)
	require.NoError(t, err)
	assert.Equal(t, sent, r.state)
}

func TestUnpermittedTriggerIsInvalidTransition(t *testing.T) {
	r := &row{state: failed}
	err := machine(r).FireCtx(context.Background(), send)
	require.ErrorIs(t, err, lifecycle.ErrInvalidTransition)
	assert.Equal(t, failed, r.state)
}

// The spike of ADR 0046, against the real library: a write that reports a compare-and-swap
// miss makes the fire call return the error and leaves the state unchanged.
func TestCompareAndSwapMissIsInvalidTransitionAndKeepsState(t *testing.T) {
	r := &row{state: draft, lost: true}
	err := machine(r).FireCtx(context.Background(), send)
	require.ErrorIs(t, err, lifecycle.ErrInvalidTransition)
	assert.Equal(t, draft, r.state)
}

func TestStorageFaultIsReturnedAsIs(t *testing.T) {
	boom := errors.New("boom")
	r := &row{state: draft, fault: boom}
	err := machine(r).FireCtx(context.Background(), send)
	require.ErrorIs(t, err, boom)
	require.NotErrorIs(t, err, lifecycle.ErrInvalidTransition)
	assert.Equal(t, draft, r.state)
}
