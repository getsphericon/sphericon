package external_test

import (
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

// A request body is a closed object at every depth (ADR 0148): a property the contract does not
// declare, at the top or nested in a Broadcast's content, is a 400 naming its path, and nothing is
// written. `segment_id` is the typo that would otherwise make a draft to all Contacts.
func TestExternalBroadcastRefusesAPropertyTheContractDoesNotDeclare(t *testing.T) {
	cases := []struct {
		name string
		edit func(body map[string]any)
		path string
	}{
		{"at the top", func(body map[string]any) { body["segment_id"] = "x" }, "segment_id"},
		{"in the content", func(body map[string]any) {
			body["content"].(map[string]any)["subjct"] = "Hello"
		}, "content.subjct"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			env := testhelper.Setup(t)
			ctx := t.Context()
			c := env.ExternalScoped(t, authorScopes...)
			edited := env.ExternalVia(t, env.ScopedBearer(t, authorScopes...), testhelper.EditBody(tc.edit))
			before, err := c.BroadcastsList(ctx, externalapi.BroadcastsListParams{})
			require.NoError(t, err)

			_, err = edited.BroadcastsCreate(ctx, &externalapi.CreateBroadcastInput{
				Name:      "Typo",
				Content:   testhelper.ExternalEmail(externalapi.EmailBroadcastContentInput{Subject: externalapi.NewOptString("Hello")}),
				SegmentId: externalapi.NilEntityId{Null: true},
			})

			after, listErr := c.BroadcastsList(ctx, externalapi.BroadcastsListParams{})

			require.NoError(t, listErr)
			refused := testhelper.ExternalRefusal(t, err, http.StatusBadRequest)
			reasons := refused.Errors.Or(nil)[tc.path]
			assert.Equal(t, []string{"unknown property: the contract does not declare it"}, reasons)
			assert.Equal(t, before.TotalItems, after.TotalItems, "a refused create stores nothing")
		})
	}
}

// A property nested in a Condition is refused the same way, and no Segment is stored.
func TestExternalSegmentRefusesAPropertyNestedInItsCondition(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := t.Context()
	scopes := []apitokens.Scope{apitokens.ScopeSegmentsRead, apitokens.ScopeSegmentsWrite}
	c := env.ExternalScoped(t, scopes...)
	edited := env.ExternalVia(t, env.ScopedBearer(t, scopes...), testhelper.EditBody(func(body map[string]any) {
		body["condition"].(map[string]any)["negate"] = true
	}))
	before, err := c.SegmentsList(ctx, externalapi.SegmentsListParams{})
	require.NoError(t, err)

	_, err = edited.SegmentsCreate(ctx, &externalapi.CreateSegmentInput{
		Name:      "Pros",
		Condition: testhelper.ConditionInput[externalapi.SegmentConditionInput](t, `{"kind":"group","combinator":"and","entries":[{"kind":"customField","key":"plan","test":{"kind":"text","operator":"eq","value":"pro"}}]}`),
	})
	after, listErr := c.SegmentsList(ctx, externalapi.SegmentsListParams{})

	require.NoError(t, listErr)
	refused := testhelper.ExternalRefusal(t, err, http.StatusBadRequest)
	reasons := refused.Errors.Or(nil)["condition.negate"]
	assert.Equal(t, []string{"unknown property: the contract does not declare it"}, reasons)
	assert.Equal(t, before.TotalItems, after.TotalItems, "a refused create stores nothing")
}

// The maps whose keys are data stay open: a Contact upsert's Custom field values and an Event's
// properties take a key the contract cannot know.
func TestExternalOpenMapsTakeAnyKey(t *testing.T) {
	env := testhelper.Setup(t, testhelper.WithClock(eventsClock))
	ctx := t.Context()
	contacts := env.ExternalScoped(t, apitokens.ScopeContactsWrite)
	events := env.ExternalScoped(t, apitokens.ScopeEventsWrite)

	upserted, upsertErr := contacts.ContactsBatchUpsert(ctx, &externalapi.UpsertContactsInput{
		Contacts: []externalapi.UpsertContactInput{{
			Email:        externalapi.NewOptNilEmailAddress("open-map@example.com"),
			CustomFields: externalapi.NewOptNilJsonObject(externalapi.JsonObject{"favourite_colour": []byte(`"teal"`)}),
		}},
	})
	eventErr := events.EventsCreate(ctx, &externalapi.RecordEventsInput{
		Events: []externalapi.EventInput{{
			SubjectId:  "s-1",
			Action:     "order_paid",
			Properties: externalapi.NewOptNilJsonObject(externalapi.JsonObject{"any_key_at_all": []byte(`1`)}),
		}},
	})

	require.NoError(t, upsertErr)
	require.NoError(t, eventErr)
	require.Len(t, upserted.Results, 1)
	result := upserted.Results[0]
	assert.Equal(t, externalapi.ContactBatchStatusCreated, result.Status, result.Error.Or(""))
}

// A failure inside an array names the item it is in: the index of the item the decoder failed at,
// counted from 0, as a segment of the path (`condition.entries.1.combinator`). The item is the one
// that fails, not the first that holds the key: a group's `combinator` is declared, the same key
// on a leaf is not.
func TestExternalSegmentNamesTheItemOfAnArrayItFailsAt(t *testing.T) {
	leaf := func(extra map[string]any) map[string]any {
		l := map[string]any{"kind": "tag", "name": "vip", "operator": "has"}
		maps.Copy(l, extra)
		return l
	}
	group := func(entries ...any) map[string]any {
		return map[string]any{"kind": "group", "combinator": "and", "entries": entries}
	}
	cases := []struct {
		name      string
		condition map[string]any
		path      string
	}{
		{
			"a key another variant declares",
			group(group(leaf(nil)), leaf(map[string]any{"combinator": "or"})),
			"condition.entries.1.combinator",
		},
		{
			"a value of the wrong type",
			group(leaf(nil), leaf(nil), leaf(map[string]any{"name": 3})),
			"condition.entries.2.name",
		},
		{
			"in a nested array",
			group(group(leaf(nil)), group(leaf(nil), leaf(map[string]any{"bogus": true}))),
			"condition.entries.1.entries.1.bogus",
		},
		{
			"inside a leaf's test",
			group(leaf(nil), map[string]any{"kind": "customField", "key": "plan", "test": map[string]any{
				"kind": "text", "operator": "eq", "value": "pro", "bogus": true,
			}}),
			"condition.entries.1.test.bogus",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			env := testhelper.Setup(t)
			ctx := t.Context()
			scopes := []apitokens.Scope{apitokens.ScopeSegmentsRead, apitokens.ScopeSegmentsWrite}
			edited := env.ExternalVia(t, env.ScopedBearer(t, scopes...), testhelper.EditBody(func(body map[string]any) {
				body["condition"] = tc.condition
			}))

			_, err := edited.SegmentsCreate(ctx, &externalapi.CreateSegmentInput{
				Name:      "Pros",
				Condition: testhelper.ConditionInput[externalapi.SegmentConditionInput](t, `{"kind":"group","combinator":"and","entries":[]}`),
			})

			refused := testhelper.ExternalRefusal(t, err, http.StatusBadRequest)
			assert.Equal(t, []string{tc.path}, slices.Collect(maps.Keys(refused.Errors.Or(nil))))
		})
	}
}
