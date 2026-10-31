package collect_test

import (
	"testing"

	"github.com/go-faster/jx"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	collectapi "github.com/getsphericon/sphericon/gen/collect"
	"github.com/getsphericon/sphericon/internal/events"
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

	res, err := env.CollectAcme(t).CollectEventsCreate(t.Context(), &collectapi.CollectEventsInput{
		Events: []collectapi.CollectEventInput{orderCompleted("9.999")},
	})

	require.NoError(t, err)
	refused, ok := res.(*collectapi.CollectEventsCreateUnprocessableEntity)
	require.Truef(t, ok, "got %T", res)
	assert.Contains(t, refused.Detail.Or(""), "total has more than 2 decimal places")
	assert.Empty(t, env.OutboxEnvelopes(t, events.NameCollected))
}

func TestTheTrackerTakesAValidOrder(t *testing.T) {
	env := testhelper.Setup(t)

	res, err := env.CollectAcme(t).CollectEventsCreate(t.Context(), &collectapi.CollectEventsInput{
		Events: []collectapi.CollectEventInput{orderCompleted("9.99")},
	})

	require.NoError(t, err)
	assert.IsType(t, &collectapi.CollectEventsCreateNoContent{}, res)
	assert.Equal(t, 1, env.OutboxCount(t, events.NameCollected, map[string]any{"action": "Order Completed"}))
}

func TestTheTrackerRefusesAnUnknownCurrencyCode(t *testing.T) {
	env := testhelper.Setup(t)
	in := orderCompleted("9.99")
	in.Properties.Value["currency"] = jx.Raw(`"QQQ"`)

	res, err := env.CollectAcme(t).CollectEventsCreate(t.Context(), &collectapi.CollectEventsInput{
		Events: []collectapi.CollectEventInput{in},
	})

	require.NoError(t, err)
	refused, ok := res.(*collectapi.CollectEventsCreateUnprocessableEntity)
	require.Truef(t, ok, "got %T", res)
	assert.Contains(t, refused.Detail.Or(""), `currency "QQQ" is not a known ISO 4217 code`)
	assert.Empty(t, env.OutboxEnvelopes(t, events.NameCollected))
}
