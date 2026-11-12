package events_test

import (
	"testing"

	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"

	"github.com/getsphericon/sphericon/internal/infra/events"
	"github.com/getsphericon/sphericon/internal/kernel/systemevent"
)

// The System events the bus records are exactly the contract's SystemEventAction, generated into
// systemevent.Actions: every bus type whose events are stored as Events, but for a customer's
// collected event, which carries the customer's own action. A new System event, on either side,
// fails here until the other side has it too, so the app labels, offers and filters every one the
// server records, and the contract names none the server does not.
func TestEverySystemEventIsTheContracts(t *testing.T) {
	recorded := lo.Filter(events.Names(), func(name string, _ int) bool {
		return !events.IsUnprojected(name) && name != events.NameCollected
	})

	assert.ElementsMatch(t, systemevent.Actions, recorded)
}
