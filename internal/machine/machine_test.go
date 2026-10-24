package machine_test

import (
	"context"
	"testing"
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

func detector(t *testing.T) (*machine.Detector, *time.Time) {
	t.Helper()
	env := testhelper.Setup(t)
	d := machine.New(env.DB, env.PrivateRelay)
	now := time.Date(2026, 10, 24, 9, 0, 0, 0, time.UTC)
	d.SetClock(func() time.Time { return now })
	return d, &now
}

// A table that cannot be read again keeps serving the last one that could: a database that hiccups
// must not turn every Private Relay hit into a human open.
func TestAFailedReloadKeepsTheLastTable(t *testing.T) {
	d, now := detector(t)
	assert.True(t, d.IsMachine(context.Background(), "", relayAddr), "the first read loads the table")

	*now = now.Add(2 * time.Hour) // the copy is stale, and the next read fails

	assert.True(t, d.IsMachine(cancelled(), "", relayAddr))
}

// With no table at all a failed read says nothing, and the read is not repeated on every hit.
func TestAFailedFirstReadIsTriedAgainAfterAWhile(t *testing.T) {
	d, now := detector(t)
	assert.False(t, d.IsMachine(cancelled(), "", relayAddr))

	assert.False(t, d.IsMachine(context.Background(), "", relayAddr), "not retried at once")

	*now = now.Add(2 * time.Minute)
	assert.True(t, d.IsMachine(context.Background(), "", relayAddr))
}
