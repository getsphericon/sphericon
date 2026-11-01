package external_test

import (
	"testing"

	"github.com/go-faster/jx"
	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	externalapi "github.com/getsphericon/sphericon/gen/external"
	"github.com/getsphericon/sphericon/internal/events"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

func orderEvent(action string, props map[string]string) externalapi.EventInput {
	raw := make(externalapi.JsonObject, len(props))
	for k, v := range props {
		raw[k] = jx.Raw(v)
	}
	return externalapi.EventInput{
		SubjectId:  "user:" + fixtures.ContactAliceEmail,
		Action:     action,
		Properties: externalapi.NewOptNilJsonObject(raw),
	}
}

func TestAnOrderCompletedIsAcceptedUnderItsOwnName(t *testing.T) {
	env := testhelper.Setup(t, testhelper.WithClock(eventsClock))
	c := env.ExternalScoped(t, "events:write")

	res, err := c.EventsCreate(t.Context(), &externalapi.RecordEventsInput{Events: []externalapi.EventInput{
		orderEvent("Order Completed", map[string]string{"order_id": `"A-1"`, "total": `19.99`, "currency": `"USD"`}),
	}})

	require.NoError(t, err)
	assert.IsType(t, &externalapi.EventsCreateNoContent{}, res)
	assert.Equal(t, 1, env.OutboxCount(t, events.NameCollected, map[string]any{"action": "Order Completed"}))
}

func TestAnAmountWithMorePlacesThanItsCurrencyOrAnUnknownCurrencyIsRefusedWithAReason(t *testing.T) {
	env := testhelper.Setup(t, testhelper.WithClock(eventsClock))
	c := env.ExternalScoped(t, "events:write")

	for name, tc := range map[string]struct {
		props  map[string]string
		reason string
	}{
		"too many places":   {map[string]string{"order_id": `"A-1"`, "total": `19.999`, "currency": `"USD"`}, "total has more than 2 decimal places"},
		"places on a yen":   {map[string]string{"order_id": `"A-1"`, "total": `10.5`, "currency": `"JPY"`}, "total has more than 0 decimal places"},
		"unknown currency":  {map[string]string{"order_id": `"A-1"`, "total": `5`, "currency": `"QQQ"`}, `currency "QQQ" is not a known ISO 4217 code`},
		"no order id":       {map[string]string{"total": `5`, "currency": `"USD"`}, "order_id is required"},
		"no total":          {map[string]string{"order_id": `"A-1"`, "currency": `"USD"`}, "total is required"},
		"total is text":     {map[string]string{"order_id": `"A-1"`, "total": `"12"`, "currency": `"USD"`}, "total must be a number"},
		"total is negative": {map[string]string{"order_id": `"A-1"`, "total": `-5`, "currency": `"USD"`}, "total must not be negative"},
		"no currency":       {map[string]string{"order_id": `"A-1"`, "total": `5`}, "currency is required"},
	} {
		res, err := c.EventsCreate(t.Context(), &externalapi.RecordEventsInput{Events: []externalapi.EventInput{orderEvent("Order Completed", tc.props)}})
		require.NoError(t, err, name)
		refused, ok := res.(*externalapi.EventsCreateUnprocessableEntity)
		require.Truef(t, ok, "%s: got %T", name, res)
		assert.Contains(t, refused.Detail.Or(""), tc.reason, name)
	}
	assert.Empty(t, env.OutboxEnvelopes(t, events.NameCollected), "nothing was published")
}

func TestAChangeWithoutACurrencyIsCheckedAgainstTheCurrencyOfItsOrder(t *testing.T) {
	env := testhelper.Setup(t, testhelper.WithClock(eventsClock))
	c := env.ExternalScoped(t, "events:write")
	res, err := c.EventsCreate(t.Context(), &externalapi.RecordEventsInput{Events: []externalapi.EventInput{
		orderEvent("Order Completed", map[string]string{"order_id": `"A-1"`, "total": `1000`, "currency": `"JPY"`}),
	}})
	require.NoError(t, err)
	require.IsType(t, &externalapi.EventsCreateNoContent{}, res)
	env.DeliverOrders(t)

	res, err = c.EventsCreate(t.Context(), &externalapi.RecordEventsInput{Events: []externalapi.EventInput{
		orderEvent("Order Refunded", map[string]string{"order_id": `"A-1"`, "total": `10.5`}),
	}})

	require.NoError(t, err)
	refused, ok := res.(*externalapi.EventsCreateUnprocessableEntity)
	require.Truef(t, ok, "got %T", res)
	assert.Contains(t, refused.Detail.Or(""), "total has more than 0 decimal places")
}

func TestAChangeWithoutACurrencyBeforeItsOrderIsCheckedAsANumber(t *testing.T) {
	env := testhelper.Setup(t, testhelper.WithClock(eventsClock))
	c := env.ExternalScoped(t, "events:write")

	for props, want := range map[string]any{`10.5`: &externalapi.EventsCreateNoContent{}, `"10"`: &externalapi.EventsCreateUnprocessableEntity{}} {
		res, err := c.EventsCreate(t.Context(), &externalapi.RecordEventsInput{Events: []externalapi.EventInput{
			orderEvent("Order Refunded", map[string]string{"order_id": `"not-yet"`, "total": props}),
		}})
		require.NoError(t, err)
		assert.IsType(t, want, res, props)
	}
}

func TestTheBatchReportsAnOrderRefusalPerItemAndTakesTheOthers(t *testing.T) {
	env := testhelper.Setup(t, testhelper.WithClock(eventsClock))
	c := env.ExternalScoped(t, "events:write")

	res, err := c.EventsBatchSubmit(t.Context(), &externalapi.RecordEventsBatchInput{Events: []externalapi.EventInput{
		orderEvent("Order Completed", map[string]string{"order_id": `"A-1"`, "total": `1.234`, "currency": `"USD"`}),
		orderEvent("Product Viewed", map[string]string{"total": `1.234`, "currency": `"QQQ"`}),
		orderEvent("Order Completed", map[string]string{"order_id": `"A-2"`, "total": `1.23`, "currency": `"USD"`}),
	}})

	require.NoError(t, err)
	batch, ok := res.(*externalapi.RecordEventsBatchResult)
	require.Truef(t, ok, "got %T", res)
	statuses := lo.Map(batch.Results, func(r externalapi.EventBatchItemResult, _ int) externalapi.EventBatchStatus { return r.Status })
	assert.Equal(t, []externalapi.EventBatchStatus{
		externalapi.EventBatchStatusFailed, externalapi.EventBatchStatusAccepted, externalapi.EventBatchStatusAccepted,
	}, statuses)
	assert.Contains(t, batch.Results[0].Error.Or(""), "total has more than 2 decimal places")
}

func TestAnOrderIsRefusedOnlyForWhatSphericonReads(t *testing.T) {
	env := testhelper.Setup(t, testhelper.WithClock(eventsClock))
	c := env.ExternalScoped(t, "events:write")

	res, err := c.EventsCreate(t.Context(), &externalapi.RecordEventsInput{Events: []externalapi.EventInput{
		orderEvent("Order Completed", map[string]string{
			"order_id": `"A-1"`, "total": `5`, "currency": `"USD"`,
			"revenue": `-1`, "discount": `0.001`, "tax": `0.001`, "shipping": `"free"`,
		}),
	}})

	require.NoError(t, err)
	assert.IsType(t, &externalapi.EventsCreateNoContent{}, res)
}
