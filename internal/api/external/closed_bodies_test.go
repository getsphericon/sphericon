package external_test

import (
	"net/http"
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
				Name:    "Typo",
				Content: testhelper.ExternalEmail(externalapi.EmailBroadcastContentInput{Subject: externalapi.NewOptString("Hello")}),
			})

			refused := testhelper.ExternalRefusal(t, err, http.StatusBadRequest)
			assert.Contains(t, refused.Errors.Or(nil), tc.path)
			after, err := c.BroadcastsList(ctx, externalapi.BroadcastsListParams{})
			require.NoError(t, err)
			assert.Equal(t, before.TotalItems, after.TotalItems)
		})
	}
}

// A property nested in a Condition is refused the same way.
func TestExternalSegmentRefusesAPropertyNestedInItsCondition(t *testing.T) {
	env := testhelper.Setup(t)
	scopes := []apitokens.Scope{apitokens.ScopeSegmentsRead, apitokens.ScopeSegmentsWrite}
	c := env.ExternalVia(t, env.ScopedBearer(t, scopes...), testhelper.EditBody(func(body map[string]any) {
		body["condition"].(map[string]any)["negate"] = true
	}))

	_, err := c.SegmentsCreate(t.Context(), &externalapi.CreateSegmentInput{
		Name:      "Pros",
		Condition: testhelper.ExternalRuleGroup(t, `{"kind":"group","combinator":"and","rules":[{"kind":"rule","field":"custom:plan","operator":"eq","value":"pro"}]}`),
	})

	refused := testhelper.ExternalRefusal(t, err, http.StatusBadRequest)
	assert.Contains(t, refused.Errors.Or(nil), "condition.negate")
}

// The maps whose keys are data stay open: a Contact's Custom field values and an Event's
// properties take a key the contract cannot know.
func TestExternalOpenMapsTakeAnyKey(t *testing.T) {
	env := testhelper.Setup(t, testhelper.WithClock(eventsClock))
	ctx := t.Context()
	contacts := env.ExternalScoped(t, apitokens.ScopeContactsWrite)
	events := env.ExternalScoped(t, apitokens.ScopeEventsWrite)

	created, err := contacts.ContactsCreate(ctx, &externalapi.CreateContactInput{
		Email:        externalapi.NewOptNilEmailAddress("open-map@example.com"),
		CustomFields: externalapi.NewOptNilJsonObject(externalapi.JsonObject{"favourite_colour": []byte(`"teal"`)}),
	})
	require.NoError(t, err)
	err = events.EventsCreate(ctx, &externalapi.RecordEventsInput{
		Events: []externalapi.EventInput{{
			SubjectId:  "s-1",
			Action:     "order_paid",
			Properties: externalapi.NewOptNilJsonObject(externalapi.JsonObject{"any_key_at_all": []byte(`1`)}),
		}},
	})

	require.NoError(t, err)
	assert.JSONEq(t, `"teal"`, string(created.CustomFields.Value["favourite_colour"]))
}
