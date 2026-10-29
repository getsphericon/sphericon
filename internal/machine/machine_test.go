package machine_test

import (
	"context"
	"testing"
	"testing/synctest"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/getsphericon/sphericon/internal/machine"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// relayAddr is inside the PrivateRelayRange fixture.
const relayAddr = "198.51.100.77"

func cancelled() context.Context {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	return ctx
}

// detector is a detector over the fixtures. A test runs it inside a testing/synctest bubble, so
// testhelper.Elapse moves the detector's clock past the reload intervals at once.
func detector(t *testing.T) *machine.Detector {
	t.Helper()
	env := testhelper.Setup(t)
	return machine.New(env.DB, env.PrivateRelay)
}

// A table that cannot be read again keeps serving the last one that could: a database that hiccups
// must not turn every Private Relay hit into a human open.
func TestAFailedReloadKeepsTheLastTable(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		d := detector(t)
		assert.True(t, d.IsMachine(context.Background(), "", relayAddr), "the first read loads the table")

		testhelper.Elapse(2 * time.Hour) // the copy is stale, and the next read fails

		assert.True(t, d.IsMachine(cancelled(), "", relayAddr))
	})
}

// With no table at all a failed read says nothing, and the read is not repeated on every hit.
func TestAFailedFirstReadIsTriedAgainAfterAWhile(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		d := detector(t)
		assert.False(t, d.IsMachine(cancelled(), "", relayAddr))

		assert.False(t, d.IsMachine(context.Background(), "", relayAddr), "not retried at once")

		testhelper.Elapse(2 * time.Minute)
		assert.True(t, d.IsMachine(context.Background(), "", relayAddr))
	})
}
