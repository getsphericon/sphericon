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
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// A date Trigger as a client writes it (ADR 0126): "3 days before" each birthday, at 10:00.
const birthdayTrigger = `{"kind":"date","field":{"kind":"birthday"},"recurrence":"yearly","offsetDays":-3,"localTime":"10:00"}`

// A date Trigger round-trips through /api by its kind: written on create, read back on the draft,
// and replaced by an update with one on a datetime Custom field.
func TestExternalDateTriggerRoundTrips(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := t.Context()
	c := env.ExternalScoped(t, apitokens.ScopeAutomationsRead, apitokens.ScopeAutomationsWrite)
	trialEnds := `{"kind":"date","field":{"kind":"customField","key":"trial_ends_at"},"recurrence":"once","offsetDays":0,"localTime":"09:30"}`

	created, err := c.AutomationsCreate(ctx, &externalapi.CreateAutomationInput{Name: "Birthday",
		Trigger: testhelper.ConditionInput[externalapi.AutomationTriggerInput](t, birthdayTrigger)})
	require.NoError(t, err)
	updated, err := c.AutomationsUpdate(ctx, &externalapi.UpdateAutomationInput{
		Revision: created.Revision,
		Trigger:  externalapi.NewOptAutomationTriggerInput(testhelper.ConditionInput[externalapi.AutomationTriggerInput](t, trialEnds)),
	}, externalapi.AutomationsUpdateParams{ID: created.ID})
	require.NoError(t, err)

	assert.JSONEq(t, birthdayTrigger, testhelper.JSONOf(t, &created.Draft.Value.Trigger))
	assert.JSONEq(t, trialEnds, testhelper.JSONOf(t, &updated.Draft.Value.Trigger))
}

// A date Trigger whose field holds no date it can recur on is refused (422): `once` on the
// birthday, which is always past, a Custom field of another type than datetime, and one the
// Workspace does not have.
func TestExternalDateTriggerRefusesAFieldWithNoOccurrence(t *testing.T) {
	for name, trigger := range map[string]string{
		"once on the birthday":    `{"kind":"date","field":{"kind":"birthday"},"recurrence":"once","offsetDays":0,"localTime":"10:00"}`,
		"a string Custom field":   `{"kind":"date","field":{"kind":"customField","key":"plan"},"recurrence":"yearly","offsetDays":0,"localTime":"10:00"}`,
		"an unknown Custom field": `{"kind":"date","field":{"kind":"customField","key":"renewal_at"},"recurrence":"yearly","offsetDays":0,"localTime":"10:00"}`,
	} {
		t.Run(name, func(t *testing.T) {
			env := testhelper.Setup(t)
			c := env.ExternalScoped(t, apitokens.ScopeAutomationsWrite)

			_, err := c.AutomationsCreate(t.Context(), &externalapi.CreateAutomationInput{Name: "Dated",
				Trigger: testhelper.ConditionInput[externalapi.AutomationTriggerInput](t, trigger)})

			testhelper.ExternalRefusal(t, err, http.StatusUnprocessableEntity)
		})
	}
}

// A date Trigger takes no Entry policy, cooldown or Enrollment key, and its offset and local time
// are bounded by the contract: each is refused with a 400 naming its path (ADR 0148).
func TestExternalDateTriggerRefusesWhatItDoesNotTakeByItsPath(t *testing.T) {
	for _, tc := range []struct {
		name  string
		field string
		value any
	}{
		{"an Entry policy", "entryPolicy", "again"},
		{"a cooldown", "cooldown", 7200},
		{"an Enrollment key", "enrollmentKey", "order_id"},
		{"an offset beyond a year", "offsetDays", 366},
		{"a local time not in HH:MM", "localTime", "9:00"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			env := testhelper.Setup(t)
			edited := env.ExternalVia(t, env.ScopedBearer(t, apitokens.ScopeAutomationsWrite), testhelper.EditBody(func(body map[string]any) {
				body["trigger"].(map[string]any)[tc.field] = tc.value
			}))

			_, err := edited.AutomationsCreate(t.Context(), &externalapi.CreateAutomationInput{Name: "Dated",
				Trigger: testhelper.ConditionInput[externalapi.AutomationTriggerInput](t, birthdayTrigger)})

			refused := testhelper.ExternalRefusal(t, err, http.StatusBadRequest)
			assert.Equal(t, []string{"trigger." + tc.field}, slices.Collect(maps.Keys(refused.Errors.Or(nil))))
		})
	}
}

// A Rule's enroll action naming an Automation whose live version starts on a date is refused, by
// its save as by its Publish (ADR 0126): only the scheduler enrolls into it.
func TestExternalRuleSaveRefusesEnrollingIntoADateTriggeredAutomation(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.ExternalScoped(t, apitokens.ScopeAutomationsRead, apitokens.ScopeAutomationsWrite)

	_, err := c.RulesCreate(t.Context(), &externalapi.CreateRuleInput{
		Name: "Enroll on a birthday", TriggerEvents: []string{"checkout"},
		Actions: []externalapi.RuleActionInput{{
			Type: externalapi.RuleActionTypeEnrollAutomation, AutomationId: externalapi.NewOptEntityId(entityIDString(fixtures.AutomationBirthdayGreetingID)),
		}},
	})

	refused := testhelper.ExternalRefusal(t, err, http.StatusUnprocessableEntity)
	assert.Contains(t, refused.Detail.Value, fixtures.AutomationBirthdayGreetingName)
}

// Publishing a date Trigger on an Automation an active Rule enrolls into is refused, naming the Rule
// (ADR 0126): the refusal holds from both sides (fixture Enroll in the greeting enrolls into the
// Jobs greeting).
func TestExternalPublishingADateTriggerAnActiveRuleEnrollsIntoIsRefusedNamingTheRule(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := t.Context()
	c := env.ExternalScoped(t, apitokens.ScopeAutomationsRead, apitokens.ScopeAutomationsWrite, apitokens.ScopeAutomationsPublish)
	id := entityIDString(fixtures.AutomationJobsGreetingID)
	_, err := c.AutomationsUpdate(ctx, &externalapi.UpdateAutomationInput{
		Revision: externalapi.Revision(env.AutomationRevision(ctx, t, fixtures.AutomationJobsGreetingID)),
		Trigger:  externalapi.NewOptAutomationTriggerInput(testhelper.ConditionInput[externalapi.AutomationTriggerInput](t, birthdayTrigger)),
	}, externalapi.AutomationsUpdateParams{ID: id})
	require.NoError(t, err)

	_, err = c.AutomationsPublish(ctx, &externalapi.RevisionInput{Revision: externalapi.Revision(env.AutomationRevision(ctx, t, fixtures.AutomationJobsGreetingID))},
		externalapi.AutomationsPublishParams{ID: id})

	refused := testhelper.ExternalRefusal(t, err, http.StatusConflict)
	assert.Contains(t, refused.Detail.Value, fixtures.RuleEnrollGreetingName)
}
