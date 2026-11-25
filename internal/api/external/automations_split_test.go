package external_test

import (
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	externalapi "github.com/getsphericon/sphericon/gen/external"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// The ids a Split of these tests is written with: the Split, its paths and a Step of one.
const (
	splitID     = "2b7c9e2f-7d3b-4b66-9f8a-4d2e3c5b6f01"
	variantID   = "2b7c9e2f-7d3b-4b66-9f8a-4d2e3c5b6f02"
	holdoutID   = "2b7c9e2f-7d3b-4b66-9f8a-4d2e3c5b6f03"
	variantStep = "2b7c9e2f-7d3b-4b66-9f8a-4d2e3c5b6f04"
)

// saveSplit creates an Automation whose root path holds the Split written as doc, then a wait.
func saveSplit(t *testing.T, c *externalapi.Client, doc string) (*externalapi.AutomationResource, error) {
	t.Helper()
	return c.AutomationsCreate(t.Context(), &externalapi.CreateAutomationInput{
		Name: "Splits", Trigger: testhelper.ExternalTrigger("signup"),
		Steps: externalapi.NewOptAutomationPathInput(testhelper.ExternalPath(
			testhelper.ExternalSplit(t, doc),
			externalapi.WaitStepInput{Seconds: 60},
		)),
	})
}

// A Split is kept as written (ADR 0124): its paths in order with their ids, weights and Steps, a
// holdout with none, and a Branch and a Split nested in its paths; the Step after it stays after it.
func TestExternalAutomationKeepsASplitsTree(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.ExternalScoped(t, stepAuthorScopes...)

	created, err := saveSplit(t, c, `{"id":"`+splitID+`","type":"split","paths":[
		{"id":"`+variantID+`","weight":40,"steps":[
			{"id":"`+variantStep+`","type":"apply_tag","tag":"variant"},
			{"type":"branch","paths":[{"condition":`+vipCondition+`,"steps":[
				{"type":"split","paths":[{"weight":1,"steps":[]},{"weight":99,"steps":[]}]}]}],"otherwise":{"steps":[]}},
			{"type":"split","paths":[{"weight":50,"steps":[{"type":"wait","seconds":5}]},{"weight":50,"steps":[]}]}
		]},
		{"id":"`+holdoutID+`","weight":60,"steps":[]}]}`)
	require.NoError(t, err)
	got, err := c.AutomationsGet(t.Context(), externalapi.AutomationsGetParams{ID: created.ID})
	require.NoError(t, err)

	steps := got.Draft.Value.Steps.Steps
	require.Len(t, steps, 2)
	require.Equal(t, externalapi.SplitStepAutomationStepSum, steps[0].OneOf.Type)
	s := steps[0].OneOf.SplitStep
	assert.EqualValues(t, splitID, s.ID)
	require.Len(t, s.Paths, 2)
	assert.EqualValues(t, variantID, s.Paths[0].ID)
	assert.EqualValues(t, 40, s.Paths[0].Weight)
	assert.EqualValues(t, holdoutID, s.Paths[1].ID)
	assert.EqualValues(t, 60, s.Paths[1].Weight)
	assert.Empty(t, s.Paths[1].Steps, "the holdout holds no Step")
	variant := s.Paths[0].Steps
	require.Len(t, variant, 3)
	assert.EqualValues(t, variantStep, variant[0].OneOf.ApplyTagStep.ID)
	require.Equal(t, externalapi.BranchStepSplitPathStepSum, variant[1].OneOf.Type)
	inBranch := variant[1].OneOf.BranchStep.Paths[0].Steps
	require.Len(t, inBranch, 1)
	require.Equal(t, externalapi.SplitStepPathStepSum, inBranch[0].OneOf.Type, "a Split nested in a Branch's path")
	assert.EqualValues(t, 99, inBranch[0].OneOf.SplitStep.Paths[1].Weight)
	require.Equal(t, externalapi.SplitStepSplitPathStepSum, variant[2].OneOf.Type, "a Split nested in a Split's path")
	nested := variant[2].OneOf.SplitStep
	assert.NotEmpty(t, nested.ID, "a nested Split is given an id")
	assert.NotEmpty(t, nested.Paths[0].ID, "a nested path is given an id")
	assert.Len(t, nested.Paths[0].Steps, 1)
	assert.Equal(t, externalapi.WaitStepAutomationStepSum, steps[1].OneOf.Type)
}

// splitPath is a path of a Split of weight, with no Steps.
func splitPath(weight int) string { return fmt.Sprintf(`{"weight":%d,"steps":[]}`, weight) }

// A Split's weights sum to 100: any other sum is refused, naming the Split.
func TestExternalAutomationRefusesASplitsWeights(t *testing.T) {
	cases := map[string]string{
		"summing to 99":  splitPath(50) + "," + splitPath(49),
		"summing to 101": splitPath(50) + "," + splitPath(51),
	}
	for name, paths := range cases {
		t.Run(name, func(t *testing.T) {
			env := testhelper.Setup(t)
			c := env.ExternalScoped(t, stepAuthorScopes...)

			_, err := saveSplit(t, c, `{"id":"`+splitID+`","type":"split","paths":[`+paths+`]}`)

			refusedNaming(t, err, splitID, "split")
		})
	}
}

// A Split has two to ten paths, each of a weight of at least 1, bounds of the contract: outside
// them the request is refused (400), naming the property.
func TestExternalAutomationRefusesASplitOutsideTheContractsBounds(t *testing.T) {
	cases := map[string]struct{ paths, property string }{
		"a weight of 0":    {splitPath(100) + "," + splitPath(0), "steps.steps.0.paths.1.weight"},
		"a negative share": {splitPath(101) + "," + splitPath(-1), "steps.steps.0.paths.1.weight"},
		"a single path":    {splitPath(100), "steps.steps.0.paths"},
		"eleven paths":     {strings.Repeat(splitPath(9)+",", 10) + splitPath(10), "steps.steps.0.paths"},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			env := testhelper.Setup(t)
			api := env.ExternalScoped(t, stepAuthorScopes...)

			_, err := saveSplit(t, api, `{"id":"`+splitID+`","type":"split","paths":[`+c.paths+`]}`)

			refused := testhelper.ExternalRefusal(t, err, http.StatusBadRequest)
			assert.Contains(t, refused.Errors.Or(nil), c.property)
		})
	}
}

// Ten paths whose weights sum to 100 are a Split the save keeps.
func TestExternalAutomationKeepsASplitOfTenPaths(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.ExternalScoped(t, stepAuthorScopes...)
	ten := strings.Repeat(`{"weight":10,"steps":[]},`, 9) + `{"weight":10,"steps":[]}`

	_, err := saveSplit(t, c, `{"type":"split","paths":[`+ten+`]}`)

	require.NoError(t, err)
}
