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

// The id a Wait until of these tests is written with.
const waitUntilID = "6c1f3a2e-8d4b-4e5f-9a7c-1b2d3e4f5a01"

// nextWorkdayAtNine is "the next weekday at 9:00" as a client writes it (ADR 0126).
const nextWorkdayAtNine = `{"id":"` + waitUntilID + `","type":"wait_until",` +
	`"until":{"form":"time_of_day","localTime":"09:00","weekdays":["monday","tuesday","wednesday","thursday","friday"]}}`

// saveWaitUntil creates an Automation whose root path holds the Wait until written as doc, then
// an email, through the client c.
func saveWaitUntil(t *testing.T, c *externalapi.Client, doc string) (*externalapi.AutomationResource, error) {
	t.Helper()
	return c.AutomationsCreate(t.Context(), &externalapi.CreateAutomationInput{
		Name: "Mornings", Trigger: testhelper.ExternalTrigger("signup"),
		Steps: externalapi.NewOptAutomationPathInput(testhelper.ExternalPath(
			testhelper.ExternalWaitUntil(t, doc),
			externalapi.EmailStepInput{Subject: "Good morning", Body: externalapi.NewOptString("<p>Good morning</p>")},
		)),
	})
}

// A Wait until round-trips through /api as {type: "wait_until", until: {form, localTime,
// weekdays}}, at the root and in a fork's path, and one written without weekdays reads back as
// every day, with none.
func TestExternalAutomationKeepsAWaitUntil(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.ExternalScoped(t, stepAuthorScopes...)
	everyDay := `{"type":"split","paths":[{"weight":50,"steps":[` +
		`{"type":"wait_until","until":{"form":"time_of_day","localTime":"18:30"}}]},{"weight":50,"steps":[]}]}`
	created, err := c.AutomationsCreate(t.Context(), &externalapi.CreateAutomationInput{
		Name: "Mornings", Trigger: testhelper.ExternalTrigger("signup"),
		Steps: externalapi.NewOptAutomationPathInput(testhelper.ExternalPath(
			testhelper.ExternalWaitUntil(t, nextWorkdayAtNine),
			testhelper.ExternalSplit(t, everyDay),
		)),
	})
	require.NoError(t, err)

	got, err := c.AutomationsGet(t.Context(), externalapi.AutomationsGetParams{ID: created.ID})

	require.NoError(t, err)
	steps := got.Draft.Value.Steps.Steps
	require.Len(t, steps, 2)
	require.Equal(t, externalapi.WaitUntilStepAutomationStepSum, steps[0].OneOf.Type)
	assert.JSONEq(t, nextWorkdayAtNine, testhelper.JSONOf(t, &steps[0].OneOf.WaitUntilStep))
	nested := steps[1].OneOf.SplitStep.Paths[0].Steps
	require.Len(t, nested, 1)
	require.Equal(t, externalapi.WaitUntilStepSplitPathStepSum, nested[0].OneOf.Type, "a Wait until in a Split's path")
	assert.JSONEq(t, `{"form":"time_of_day","localTime":"18:30","weekdays":[]}`, testhelper.JSONOf(t, &nested[0].OneOf.WaitUntilStep.Until))
}

// A repeated weekday is refused by the save (422), naming the Step.
func TestExternalAutomationRefusesAWaitUntilWithARepeatedWeekday(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.ExternalScoped(t, stepAuthorScopes...)

	_, err := saveWaitUntil(t, c, `{"id":"`+waitUntilID+`","type":"wait_until",`+
		`"until":{"form":"time_of_day","localTime":"09:00","weekdays":["monday","monday"]}}`)

	detail := refusedNaming(t, err, waitUntilID, "wait_until")
	assert.Contains(t, detail, "repeated weekday")
}

// What the contract does not admit in a Wait until is refused with a 400 naming its path (ADR
// 0148): a field its form does not hold, a local time not written HH:MM and a day that is no
// weekday.
func TestExternalAutomationRefusesAWaitUntilOutsideTheContract(t *testing.T) {
	for _, tc := range []struct {
		name string
		edit func(until map[string]any)
		path string
	}{
		{"an unknown field", func(until map[string]any) { until["offsetDays"] = 3 }, "steps.steps.0.until.offsetDays"},
		{"a local time not written HH:MM", func(until map[string]any) { until["localTime"] = "9:00" }, "steps.steps.0.until.localTime"},
		{"a day that is no weekday", func(until map[string]any) { until["weekdays"] = []any{"funday"} }, "steps.steps.0.until.weekdays.0"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			env := testhelper.Setup(t)
			edited := env.ExternalVia(t, env.ScopedBearer(t, apitokens.ScopeAutomationsWrite), testhelper.EditBody(func(body map[string]any) {
				step := body["steps"].(map[string]any)["steps"].([]any)[0].(map[string]any)
				tc.edit(step["until"].(map[string]any))
			}))

			_, err := saveWaitUntil(t, edited, nextWorkdayAtNine)

			refused := testhelper.ExternalRefusal(t, err, http.StatusBadRequest)
			assert.Equal(t, []string{tc.path}, slices.Collect(maps.Keys(refused.Errors.Or(nil))))
		})
	}
}
