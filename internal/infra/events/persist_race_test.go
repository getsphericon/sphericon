package events_test

import (
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/event"
	"github.com/getsphericon/sphericon/internal/app/entclient"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/infra/events"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// Retries of one clientEventId that arrive at the same moment, each published in a transaction
// of its own on a connection of its own, are one Event, stored and published once: the unique
// source_id is the gate, and the transactions that lose the race publish nothing.
func TestPublishingOneClientEventIdConcurrentlyStoresAndPublishesItOnce(t *testing.T) {
	sqlDB := testhelper.RealDB(t)
	sqlDB.SetMaxOpenConns(8)
	client := entclient.Core.New(sqlDB)
	bus := testhelper.BusOn(t, sqlDB)
	const action = "race_client_event"

	var wg sync.WaitGroup
	errs := make([]error, 8)
	for i := range errs {
		wg.Go(func() {
			errs[i] = bus.WithinTx(t.Context(), func(_ *ent.Client, pub events.Publisher) error {
				return pub.Publish(t.Context(), &events.CollectedEvent{
					WorkspaceID: fixtures.AcmeID, VisitorID: "v-race", Action: action,
					DedupID: "collect:race:evt-1",
				})
			})
		})
	}
	wg.Wait()

	for _, err := range errs {
		require.NoError(t, err)
	}
	n, err := client.Event.Query().Where(event.Action(action)).Count(t.Context())
	require.NoError(t, err)
	assert.Equal(t, 1, n, "stored once")
	published := testhelper.QueuedEnvelopesOn(t, sqlDB, events.NameCollected)
	assert.Len(t, published, 1, "published once")
}
