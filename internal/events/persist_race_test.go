package events_test

import (
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/event"
	"github.com/getsphericon/sphericon/internal/db"
	"github.com/getsphericon/sphericon/internal/events"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// Two retries of one clientEventId that both passed the collect-side check before either was
// persisted leave two envelopes on the outbox; persisting them at the same moment, on separate
// connections, still writes one Event: the unique source_id is the gate.
func TestPersistingTwoEnvelopesOfOneClientEventIdConcurrentlyWritesOneEvent(t *testing.T) {
	sqlDB := testhelper.RealDB(t)
	sqlDB.SetMaxOpenConns(8)
	client := db.NewEntClient(sqlDB)
	bus := events.New(sqlDB)
	const action = "race_client_event"
	for range 2 {
		require.NoError(t, bus.WithinTx(t.Context(), func(_ *ent.Client, pub events.Publisher) error {
			return pub.Publish(t.Context(), &events.CollectedEvent{
				WorkspaceID: fixtures.AcmeID, VisitorID: "v-race", Action: action,
				DedupID: "collect:race:evt-1",
			})
		}))
	}
	envelopes, err := testhelper.OutboxEnvelopesOn(t.Context(), client, events.NameCollected)
	require.NoError(t, err)
	require.Len(t, envelopes, 2)

	var wg sync.WaitGroup
	errs := make([]error, 8)
	for i := range errs {
		wg.Go(func() { errs[i] = events.Persist(t.Context(), client, envelopes[i%2]) })
	}
	wg.Wait()

	for _, err := range errs {
		require.NoError(t, err)
	}
	n, err := client.Event.Query().Where(event.Action(action)).Count(t.Context())
	require.NoError(t, err)
	assert.Equal(t, 1, n)
}
