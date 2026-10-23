package collect_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent/event"
	collectapi "github.com/getsphericon/sphericon/gen/collect"
	"github.com/getsphericon/sphericon/internal/events"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

func clientEvent(id, action string) collectapi.CollectEventInput {
	return collectapi.CollectEventInput{
		VisitorId:     "v-idem",
		Action:        action,
		ClientEventId: collectapi.NewOptNilString(id),
	}
}

// persistOutbox hands every envelope on the outbox to the persist subscriber, as the
// router would (no router runs under txdb).
func persistOutbox(t *testing.T, env *testhelper.TestEnv) {
	t.Helper()
	envelopes := env.OutboxEnvelopes(t)
	for i := range envelopes {
		require.NoError(t, events.Persist(t.Context(), env.DB, envelopes[i]))
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

	res, err := c.CollectEventsCreate(ctx, batch)
	require.NoError(t, err)
	require.IsType(t, &collectapi.CollectEventsCreateNoContent{}, res)
	persistOutbox(t, env)

	// The client never saw the answer and sends the batch again.
	res, err = c.CollectEventsCreate(ctx, batch)
	require.NoError(t, err)
	assert.IsType(t, &collectapi.CollectEventsCreateNoContent{}, res, "a repeat is accepted")
	persistOutbox(t, env)

	assert.Equal(t, 1, countAction(t, env, "idem_after_delivery"))
	assert.Len(t, env.OutboxEnvelopes(t, events.NameCollected), 1, "a repeat publishes nothing")
}

func TestARepeatSentBeforeDeliveryIsStillCountedOnce(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.CollectAcme(t)
	batch := &collectapi.CollectEventsInput{Events: []collectapi.CollectEventInput{clientEvent("evt-2", "idem_before_delivery")}}

	for range 2 {
		_, err := c.CollectEventsCreate(t.Context(), batch)
		require.NoError(t, err)
	}
	persistOutbox(t, env)

	assert.Equal(t, 1, countAction(t, env, "idem_before_delivery"))
}

func TestClientEventIdsAreUniquePerWorkspace(t *testing.T) {
	env := testhelper.Setup(t)
	batch := &collectapi.CollectEventsInput{Events: []collectapi.CollectEventInput{clientEvent("shared-id", "idem_two_workspaces")}}

	for _, key := range []string{fixtures.AcmeCollectKey, fixtures.GlobexCollectKey} {
		_, err := env.CollectWithKey(t, key).CollectEventsCreate(t.Context(), batch)
		require.NoError(t, err)
	}
	persistOutbox(t, env)

	assert.Equal(t, 2, countAction(t, env, "idem_two_workspaces"))
}

func TestEventsWithoutAClientEventIdAreNotDeduplicated(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.CollectAcme(t)
	batch := &collectapi.CollectEventsInput{Events: []collectapi.CollectEventInput{{VisitorId: "v-idem", Action: "idem_no_id"}}}

	for range 2 {
		_, err := c.CollectEventsCreate(t.Context(), batch)
		require.NoError(t, err)
	}
	persistOutbox(t, env)

	assert.Equal(t, 2, countAction(t, env, "idem_no_id"))
}
