package sendingdomains_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/internal/domain/sending/sendingdomains"
	"github.com/getsphericon/sphericon/internal/kernel/lifecycle"
)

// Every move of the DKIM verification, from each state on each trigger: where a move leads, and
// where the trigger changes nothing and writes nothing (#1245).
func TestDKIMMachineMoves(t *testing.T) {
	const stays = sendingdomains.DKIMState("")
	for _, tc := range []struct {
		from    sendingdomains.DKIMState
		trigger sendingdomains.DKIMTrigger
		to      sendingdomains.DKIMState
	}{
		{sendingdomains.DKIMUnverified, sendingdomains.DKIMFound, sendingdomains.DKIMVerified},
		{sendingdomains.DKIMUnverified, sendingdomains.DKIMMissing, stays},
		{sendingdomains.DKIMUnverified, sendingdomains.DKIMSelectorMoved, stays},
		{sendingdomains.DKIMVerified, sendingdomains.DKIMFound, stays},
		{sendingdomains.DKIMVerified, sendingdomains.DKIMMissing, sendingdomains.DKIMLost},
		{sendingdomains.DKIMVerified, sendingdomains.DKIMSelectorMoved, sendingdomains.DKIMLost},
		{sendingdomains.DKIMLost, sendingdomains.DKIMFound, sendingdomains.DKIMVerified},
		{sendingdomains.DKIMLost, sendingdomains.DKIMMissing, stays},
		{sendingdomains.DKIMLost, sendingdomains.DKIMSelectorMoved, stays},
	} {
		t.Run(fmt.Sprintf("%s on %s", tc.from, tc.trigger), func(t *testing.T) {
			var written sendingdomains.DKIMState
			machine := sendingdomains.DKIMMachine(lifecycle.Storage[sendingdomains.DKIMState]{
				Read: func(context.Context) (sendingdomains.DKIMState, error) { return tc.from, nil },
				Write: func(_ context.Context, _, to sendingdomains.DKIMState) (bool, error) {
					written = to
					return true, nil
				},
			})

			err := machine.FireCtx(t.Context(), tc.trigger)

			require.NoError(t, err)
			assert.Equal(t, tc.to, written)
		})
	}
}

// A move whose write misses (the domain changed since it was read) is a refused move.
func TestDKIMMachineLostSwapIsInvalidTransition(t *testing.T) {
	machine := sendingdomains.DKIMMachine(lifecycle.Storage[sendingdomains.DKIMState]{
		Read:  func(context.Context) (sendingdomains.DKIMState, error) { return sendingdomains.DKIMVerified, nil },
		Write: func(context.Context, sendingdomains.DKIMState, sendingdomains.DKIMState) (bool, error) { return false, nil },
	})

	err := machine.FireCtx(t.Context(), sendingdomains.DKIMMissing)

	require.ErrorIs(t, err, lifecycle.ErrInvalidTransition)
}
