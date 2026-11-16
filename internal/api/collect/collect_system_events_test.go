package collect_test

import (
	"net/http"
	"testing"

	"github.com/go-faster/jx"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent/customfield"
	collectapi "github.com/getsphericon/sphericon/gen/collect"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/kernel/systemevent"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// An Identify records contact.field_changed for the defined Custom field whose stored value it
// changes (#897). It is a claim (ADR 0171): it sets a value where there was none, and never replaces
// or clears one, so nothing after the first records anything. A trait no Custom field is defined for
// is dropped, and no definition is made.
func TestIdentifyRecordsCustomFieldChanges(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.CollectAcme(t)
	identify := func(plan string) {
		t.Helper()
		_, err := c.CollectIdentifyCreate(t.Context(), &collectapi.CollectIdentifyInput{
			VisitorId: "dev-plan",
			Email:     collectapi.NewOptNilEmailAddress("planner@example.com"),
			Traits:    collectapi.NewOptNilJsonObject(collectapi.JsonObject{"plan": jx.Raw(plan), "undefined_trait": jx.Raw(`"x"`)}),
		})
		require.NoError(t, err)
	}

	identify(`"pro"`)
	afterSet := env.FieldChanges(t)
	identify(`"pro"`)
	identify(`"team"`)
	identify(`null`)
	afterRest := env.FieldChanges(t)

	assert.Equal(t, []map[string]any{{"key": "plan", "value": "pro"}}, afterSet, "a value where there was none")
	assert.Equal(t, afterSet, afterRest, "a claim neither repeats, replaces nor clears")
	assert.False(t, env.DB.CustomField.Query().Where(customfield.WorkspaceID(fixtures.AcmeID), customfield.Key("undefined_trait")).ExistX(t.Context()),
		"no Custom field is defined through the collect key")
}

// A customer's Event never claims an Enrollment's action.
func TestCollectRefusesAutomationActions(t *testing.T) {
	env := testhelper.Setup(t)

	err := env.CollectAcme(t).CollectEventsCreate(t.Context(), &collectapi.CollectEventsInput{
		Events: []collectapi.CollectEventInput{{VisitorId: "v-1", Action: systemevent.AutomationCompleted}},
	})
	refused := testhelper.CollectRefusal(t, err, http.StatusUnprocessableEntity)
	assert.Contains(t, refused.Detail.Or(""), "reserved")
}
