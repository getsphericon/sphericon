package external_test

import (
	"net/http"
	"testing"

	"github.com/go-faster/jx"
	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	externalapi "github.com/getsphericon/sphericon/gen/external"
	"github.com/getsphericon/sphericon/internal/domain/identity/apitokens"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/infra/events"
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
	c := env.ExternalScoped(t, apitokens.ScopeEventsWrite)

	err := c.EventsCreate(t.Context(), &externalapi.RecordEventsInput{Events: []externalapi.EventInput{
		orderEvent("Order Completed", map[string]string{"order_id": `"A-1"`, "total": `19.99`, "currency": `"USD"`}),
	}})

	require.NoError(t, err)
	assert.Equal(t, 1, env.QueuedCount(t, events.NameCollected, map[string]any{"action": "Order Completed"}))
}

func TestAnAmountWithMorePlacesThanItsCurrencyOrAnUnknownCurrencyIsRefusedWithAReason(t *testing.T) {
	env := testhelper.Setup(t, testhelper.WithClock(eventsClock))
	c := env.ExternalScoped(t, apitokens.ScopeEventsWrite)

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
		err := c.EventsCreate(t.Context(), &externalapi.RecordEventsInput{Events: []externalapi.EventInput{orderEvent("Order Completed", tc.props)}})
		refused := testhelper.ExternalRefusal(t, err, http.StatusUnprocessableEntity, name)
		assert.Contains(t, refused.Detail.Or(""), tc.reason, name)
	}
	assert.Empty(t, env.QueuedEnvelopes(t, events.NameCollected), "nothing was published")
}

func TestAChangeWithoutACurrencyIsCheckedAgainstTheCurrencyOfItsOrder(t *testing.T) {
	env := testhelper.Setup(t, testhelper.WithClock(eventsClock))
	c := env.ExternalScoped(t, apitokens.ScopeEventsWrite)

	// fixture-order-1 is Alice's Order OrderAcme, in USD.
	err := c.EventsCreate(t.Context(), &externalapi.RecordEventsInput{Events: []externalapi.EventInput{
		orderEvent("Order Refunded", map[string]string{"order_id": `"fixture-order-1"`, "total": `10.555`}),
	}})

	refused := testhelper.ExternalRefusal(t, err, http.StatusUnprocessableEntity)
	assert.Contains(t, refused.Detail.Or(""), "total has more than 2 decimal places, which USD allows")
}

func TestAChangeWithoutACurrencyBeforeItsOrderIsCheckedAsANumber(t *testing.T) {
	env := testhelper.Setup(t, testhelper.WithClock(eventsClock))
	c := env.ExternalScoped(t, apitokens.ScopeEventsWrite)

	refund := func(total string) error {
		return c.EventsCreate(t.Context(), &externalapi.RecordEventsInput{Events: []externalapi.EventInput{
			orderEvent("Order Refunded", map[string]string{"order_id": `"not-yet"`, "total": total}),
		}})
	}

	require.NoError(t, refund(`10.5`), "a number is a total")
	testhelper.ExternalRefusal(t, refund(`"10"`), http.StatusUnprocessableEntity, "a string is not")
}

func TestTheBatchReportsAnOrderRefusalPerItemAndTakesTheOthers(t *testing.T) {
	env := testhelper.Setup(t, testhelper.WithClock(eventsClock))
	c := env.ExternalScoped(t, apitokens.ScopeEventsWrite)

	batch, err := c.EventsBatchSubmit(t.Context(), &externalapi.RecordEventsBatchInput{Events: []externalapi.EventInput{
		orderEvent("Order Completed", map[string]string{"order_id": `"A-1"`, "total": `1.234`, "currency": `"USD"`}),
		orderEvent("Product Viewed", map[string]string{"total": `1.234`, "currency": `"QQQ"`}),
		orderEvent("Order Completed", map[string]string{"order_id": `"A-2"`, "total": `1.23`, "currency": `"USD"`}),
	}})

	require.NoError(t, err)
	statuses := lo.Map(batch.Results, func(r externalapi.EventBatchItemResult, _ int) externalapi.EventBatchStatus { return r.Status })
	assert.Equal(t, []externalapi.EventBatchStatus{
		externalapi.EventBatchStatusFailed, externalapi.EventBatchStatusAccepted, externalapi.EventBatchStatusAccepted,
	}, statuses)
	assert.Contains(t, batch.Results[0].Error.Or(""), "total has more than 2 decimal places")
}

func TestAnOrderIsRefusedOnlyForWhatSphericonReads(t *testing.T) {
	env := testhelper.Setup(t, testhelper.WithClock(eventsClock))
	c := env.ExternalScoped(t, apitokens.ScopeEventsWrite)

	err := c.EventsCreate(t.Context(), &externalapi.RecordEventsInput{Events: []externalapi.EventInput{
		orderEvent("Order Completed", map[string]string{
			"order_id": `"A-1"`, "total": `5`, "currency": `"USD"`,
			"revenue": `-1`, "discount": `0.001`, "tax": `0.001`, "shipping": `"free"`,
		}),
	}})

	require.NoError(t, err)
}
