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

// An Identify records contact.field_changed for each Custom field whose stored value it changes
// (#897): a value set where there was none, a new value, or a clear, which carries no value. One
// repeating the stored value records nothing.
func TestIdentifyRecordsCustomFieldChanges(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.CollectAcme(t)
	identify := func(plan string) {
		t.Helper()
		res, err := c.CollectIdentifyCreate(t.Context(), &collectapi.CollectIdentifyInput{
			VisitorId: "dev-plan",
			Email:     collectapi.NewOptNilEmailAddress("planner@example.com"),
			Traits:    collectapi.NewOptNilJsonObject(collectapi.JsonObject{"plan": jx.Raw(plan)}),
		})
		require.NoError(t, err)
		require.IsType(t, &collectapi.CollectOkResponse{}, res)
	}
	changes := func() []map[string]any {
		messages := env.Outbox(t, events.NameContactFieldChanged)
		out := make([]map[string]any, 0, len(messages))
		for i := range messages {
			out = append(out, map[string]any{"key": messages[i].Data["key"], "value": messages[i].Data["value"]})
		}
		return out
	}

	identify(`"pro"`)
	assert.Equal(t, []map[string]any{{"key": "plan", "value": "pro"}}, changes(), "a value where there was none")

	identify(`"pro"`)
	assert.Len(t, changes(), 1, "the same value again records nothing")

	identify(`"team"`)
	identify(`null`)
	assert.Equal(t, []map[string]any{
		{"key": "plan", "value": "pro"},
		{"key": "plan", "value": "team"},
		{"key": "plan", "value": nil},
	}, changes(), "a new value, then a clear with no value")

	identify(`null`)
	assert.Len(t, changes(), 3, "clearing a cleared field records nothing")
}

// A customer's Event never claims an Enrollment's action.
func TestCollectRefusesAutomationActions(t *testing.T) {
	env := testhelper.Setup(t)

	res, err := env.CollectAcme(t).CollectEventsCreate(t.Context(), &collectapi.CollectEventsInput{
		Events: []collectapi.CollectEventInput{{VisitorId: "v-1", Action: "automation.completed"}},
	})
	require.NoError(t, err)
	refused, isRefused := res.(*collectapi.CollectEventsCreateUnprocessableEntity)
	require.Truef(t, isRefused, "got %T", res)
	assert.Contains(t, refused.Detail.Or(""), "reserved")
}
