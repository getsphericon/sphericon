package collect_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent/event"
	collectapi "github.com/getsphericon/sphericon/gen/collect"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/infra/events"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

func clientEvent(id, action string) collectapi.CollectEventInput {
	return collectapi.CollectEventInput{
		VisitorId:     "v-idem",
		Action:        action,
		ClientEventId: collectapi.NewOptNilString(id),
	}
}

func countAction(t *testing.T, env *testhelper.TestEnv, action string) int {
	t.Helper()
	n, err := env.DB.Event.Query().Where(event.Action(action)).Count(t.Context())
	require.NoError(t, err)
	return n
}

func TestARetriedEventIsCountedOnce(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.CollectAcme(t)
	ctx := context.Background()
	batch := &collectapi.CollectEventsInput{Events: []collectapi.CollectEventInput{clientEvent("evt-1", "idem_after_delivery")}}

	err := c.CollectEventsCreate(ctx, batch)
	require.NoError(t, err)

	// The client never saw the answer and sends the batch again.
	err = c.CollectEventsCreate(ctx, batch)
	require.NoError(t, err, "a repeat is accepted")

	assert.Equal(t, 1, countAction(t, env, "idem_after_delivery"))
	assert.Len(t, env.QueuedEnvelopes(t, events.NameCollected), 1, "a repeat publishes nothing")
}

func TestARepeatSentBeforeDeliveryIsStillCountedOnce(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.CollectAcme(t)
	batch := &collectapi.CollectEventsInput{Events: []collectapi.CollectEventInput{clientEvent("evt-2", "idem_before_delivery")}}

	for range 2 {
		err := c.CollectEventsCreate(t.Context(), batch)
		require.NoError(t, err)
	}

	assert.Equal(t, 1, countAction(t, env, "idem_before_delivery"))
}

func TestClientEventIdsAreUniquePerWorkspace(t *testing.T) {
	env := testhelper.Setup(t)
	batch := &collectapi.CollectEventsInput{Events: []collectapi.CollectEventInput{clientEvent("shared-id", "idem_two_workspaces")}}

	for _, key := range []string{fixtures.AcmeCollectKey, fixtures.GlobexCollectKey} {
		err := env.CollectWithKey(t, key).CollectEventsCreate(t.Context(), batch)
		require.NoError(t, err)
	}

	assert.Equal(t, 2, countAction(t, env, "idem_two_workspaces"))
}

func TestEventsWithoutAClientEventIdAreNotDeduplicated(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.CollectAcme(t)
	batch := &collectapi.CollectEventsInput{Events: []collectapi.CollectEventInput{{VisitorId: "v-idem", Action: "idem_no_id"}}}

	for range 2 {
		err := c.CollectEventsCreate(t.Context(), batch)
		require.NoError(t, err)
	}

	assert.Equal(t, 2, countAction(t, env, "idem_no_id"))
}
