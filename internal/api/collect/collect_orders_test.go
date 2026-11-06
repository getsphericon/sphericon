package collect_test

import (
	"net/http"
	"testing"

	"github.com/go-faster/jx"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	collectapi "github.com/getsphericon/sphericon/gen/collect"
	"github.com/getsphericon/sphericon/internal/infra/events"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

func orderCompleted(total string) collectapi.CollectEventInput {
	return collectapi.CollectEventInput{
		VisitorId: "v-1",
		Action:    "Order Completed",
		Properties: collectapi.NewOptNilJsonObject(collectapi.JsonObject{
			"order_id": jx.Raw(`"T-1"`), "total": jx.Raw(total), "currency": jx.Raw(`"USD"`),
		}),
	}
}

func TestTheTrackerRefusesAnOrderAmountWithTooManyPlaces(t *testing.T) {
	env := testhelper.Setup(t)

	err := env.CollectAcme(t).CollectEventsCreate(t.Context(), &collectapi.CollectEventsInput{
		Events: []collectapi.CollectEventInput{orderCompleted("9.999")},
	})

	refused := testhelper.CollectRefusal(t, err, http.StatusUnprocessableEntity)
	assert.Contains(t, refused.Detail.Or(""), "total has more than 2 decimal places")
	assert.Empty(t, env.QueuedEnvelopes(t, events.NameCollected))
}

func TestTheTrackerTakesAValidOrder(t *testing.T) {
	env := testhelper.Setup(t)

	err := env.CollectAcme(t).CollectEventsCreate(t.Context(), &collectapi.CollectEventsInput{
		Events: []collectapi.CollectEventInput{orderCompleted("9.99")},
	})

	require.NoError(t, err)
	assert.Equal(t, 1, env.QueuedCount(t, events.NameCollected, map[string]any{"action": "Order Completed"}))
}

func TestTheTrackerRefusesAnOrderCompletedWithoutACurrency(t *testing.T) {
	env := testhelper.Setup(t)
	in := orderCompleted("9.99")
	delete(in.Properties.Value, "currency")

	err := env.CollectAcme(t).CollectEventsCreate(t.Context(), &collectapi.CollectEventsInput{
		Events: []collectapi.CollectEventInput{in},
	})

	refused := testhelper.CollectRefusal(t, err, http.StatusUnprocessableEntity)
	assert.Contains(t, refused.Detail.Or(""), "currency is required")
	assert.Empty(t, env.QueuedEnvelopes(t, events.NameCollected))
}

func TestTheTrackerRefusesAnUnknownCurrencyCode(t *testing.T) {
	env := testhelper.Setup(t)
	in := orderCompleted("9.99")
	in.Properties.Value["currency"] = jx.Raw(`"QQQ"`)

	err := env.CollectAcme(t).CollectEventsCreate(t.Context(), &collectapi.CollectEventsInput{
		Events: []collectapi.CollectEventInput{in},
	})

	refused := testhelper.CollectRefusal(t, err, http.StatusUnprocessableEntity)
	assert.Contains(t, refused.Detail.Or(""), `currency "QQQ" is not a known ISO 4217 code`)
	assert.Empty(t, env.QueuedEnvelopes(t, events.NameCollected))
}
