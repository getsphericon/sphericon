package external_test

import (
	"encoding/json"
	"maps"
	"net/http"
	"slices"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	externalapi "github.com/getsphericon/sphericon/gen/external"
	"github.com/getsphericon/sphericon/internal/domain/identity/apitokens"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// An Event filter as a client writes it.
const paidOrderFilter = `{"kind":"group","combinator":"and","entries":[
	{"kind":"property","key":"status","test":{"kind":"text","operator":"eq","value":"paid"}}
]}`

// The Trigger round-trips through /api as {kind: "event", action, filter} (ADR 0133).
func TestExternalTriggerIsReadBackAsAnEventMatcher(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := t.Context()
	c := env.ExternalScoped(t, apitokens.ScopeAutomationsRead, apitokens.ScopeAutomationsWrite)

	created, err := c.AutomationsCreate(ctx, &externalapi.CreateAutomationInput{Name: "Paid orders",
		Trigger: testhelper.ExternalTrigger("order.completed", testhelper.ConditionInput[externalapi.EventFilterInput](t, paidOrderFilter))})
	require.NoError(t, err)
	read, err := c.AutomationsGet(ctx, externalapi.AutomationsGetParams{ID: created.ID})
	require.NoError(t, err)

	assert.JSONEq(t, `{"kind":"event","action":"order.completed","filter":`+paidOrderFilter+`}`, testhelper.JSONOf(t, &read.Draft.Value.Trigger))
}

// A test of the Contact in a filter, and a field the Trigger does not declare, are refused with a
// 400 naming the path (ADR 0148, ADR 0181).
func TestExternalTriggerRefusesWhatItDoesNotHoldByItsPath(t *testing.T) {
	for _, tc := range []struct {
		name string
		edit func(trigger map[string]any)
		path string
	}{
		{"a test of a Contact field in the filter", func(trigger map[string]any) {
			var filter map[string]any
			require.NoError(t, json.Unmarshal([]byte(`{"kind":"group","combinator":"and","entries":[
				{"kind":"attribute","attribute":"email","test":{"kind":"text","operator":"eq","value":"x"}}
			]}`), &filter))
			trigger["filter"] = filter
		}, "trigger.filter.entries.0"},
		{"an unknown field of the Trigger", func(trigger map[string]any) { trigger["event"] = "order.completed" }, "trigger.event"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			env := testhelper.Setup(t)
			edited := env.ExternalVia(t, env.ScopedBearer(t, apitokens.ScopeAutomationsWrite), testhelper.EditBody(func(body map[string]any) {
				tc.edit(body["trigger"].(map[string]any))
			}))

			_, err := edited.AutomationsCreate(t.Context(), &externalapi.CreateAutomationInput{Name: "Misplaced", Trigger: testhelper.ExternalTrigger("order.completed")})

			refused := testhelper.ExternalRefusal(t, err, http.StatusBadRequest)
			assert.Equal(t, []string{tc.path}, slices.Collect(maps.Keys(refused.Errors.Or(nil))))
		})
	}
}
