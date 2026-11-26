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

// threeDaysBeforeTrialEnds is "3 days before trial_ends_at, at 10:00" as a client writes it (ADR
// 0126): the date Trigger's model beside the form.
const threeDaysBeforeTrialEnds = `{"id":"` + waitUntilID + `","type":"wait_until",` +
	`"until":{"form":"date","field":{"kind":"customField","key":"trial_ends_at"},"recurrence":"once","offsetDays":-3,"localTime":"10:00"}}`

// A Wait until on a date round-trips through /api as {form: "date", field, recurrence, offsetDays,
// localTime}, the field a typed reference: a Custom field by its key, or the birthday.
func TestExternalAutomationKeepsAWaitUntilOnADate(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.ExternalScoped(t, stepAuthorScopes...)
	nextBirthday := `{"type":"split","paths":[{"weight":50,"steps":[` +
		`{"type":"wait_until","until":{"form":"date","field":{"kind":"birthday"},"recurrence":"yearly","offsetDays":0,"localTime":"10:00"}}]},` +
		`{"weight":50,"steps":[]}]}`
	created, err := c.AutomationsCreate(t.Context(), &externalapi.CreateAutomationInput{
		Name: "Trial ending", Trigger: testhelper.ExternalTrigger("signup"),
		Steps: externalapi.NewOptAutomationPathInput(testhelper.ExternalPath(
			testhelper.ExternalWaitUntil(t, threeDaysBeforeTrialEnds),
			testhelper.ExternalSplit(t, nextBirthday),
		)),
	})
	require.NoError(t, err)

	got, err := c.AutomationsGet(t.Context(), externalapi.AutomationsGetParams{ID: created.ID})

	require.NoError(t, err)
	steps := got.Draft.Value.Steps.Steps
	require.Len(t, steps, 2)
	require.Equal(t, externalapi.WaitUntilStepAutomationStepSum, steps[0].OneOf.Type)
	assert.JSONEq(t, threeDaysBeforeTrialEnds, testhelper.JSONOf(t, &steps[0].OneOf.WaitUntilStep))
	nested := steps[1].OneOf.SplitStep.Paths[0].Steps
	require.Len(t, nested, 1)
	assert.JSONEq(t, `{"form":"date","field":{"kind":"birthday"},"recurrence":"yearly","offsetDays":0,"localTime":"10:00"}`,
		testhelper.JSONOf(t, &nested[0].OneOf.WaitUntilStep.Until))
}

// The date form is refused at save as a date Trigger is (422, naming the Step): a Custom field the
// catalogue lacks, one of another type than datetime, and `once` on the birthday, which is always
// past (decision 5 of #1613).
func TestExternalAutomationRefusesAWaitUntilOnADateItCannotReach(t *testing.T) {
	for _, tc := range []struct {
		name, field, recurrence, want string
	}{
		{"an unknown Custom field", `{"kind":"customField","key":"no_such_field"}`, "once", "no_such_field"},
		{"a Custom field that is not datetime", `{"kind":"customField","key":"plan"}`, "once", "not a datetime"},
		{"once on the birthday", `{"kind":"birthday"}`, "once", "only yearly"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			env := testhelper.Setup(t)
			c := env.ExternalScoped(t, stepAuthorScopes...)

			_, err := saveWaitUntil(t, c, `{"id":"`+waitUntilID+`","type":"wait_until","until":{"form":"date","field":`+tc.field+
				`,"recurrence":"`+tc.recurrence+`","offsetDays":0,"localTime":"10:00"}}`)

			detail := refusedNaming(t, err, waitUntilID, "wait_until")
			assert.Contains(t, detail, tc.want)
		})
	}
}

// What the contract does not admit in the date form is refused with a 400 naming its path (ADR
// 0148): an offset beyond a year, a local time not written HH:MM, a field of no known kind and a
// field the form does not hold.
func TestExternalAutomationRefusesAWaitUntilOnADateOutsideTheContract(t *testing.T) {
	for _, tc := range []struct {
		name string
		edit func(until map[string]any)
		path string
	}{
		{"an offset beyond a year", func(until map[string]any) { until["offsetDays"] = 366 }, "steps.steps.0.until.offsetDays"},
		{"a local time not written HH:MM", func(until map[string]any) { until["localTime"] = "10am" }, "steps.steps.0.until.localTime"},
		{"a field of no known kind", func(until map[string]any) { until["field"] = map[string]any{"kind": "anniversary"} }, "steps.steps.0.until.field"},
		{"a field the form does not hold", func(until map[string]any) { until["weekdays"] = []any{"monday"} }, "steps.steps.0.until.weekdays"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			env := testhelper.Setup(t)
			edited := env.ExternalVia(t, env.ScopedBearer(t, apitokens.ScopeAutomationsWrite), testhelper.EditBody(func(body map[string]any) {
				step := body["steps"].(map[string]any)["steps"].([]any)[0].(map[string]any)
				tc.edit(step["until"].(map[string]any))
			}))

			_, err := saveWaitUntil(t, edited, threeDaysBeforeTrialEnds)

			refused := testhelper.ExternalRefusal(t, err, http.StatusBadRequest)
			assert.Equal(t, []string{tc.path}, slices.Collect(maps.Keys(refused.Errors.Or(nil))))
		})
	}
}
