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

// The ids a Branch of these tests is written with: the Branch, its paths and a Step of one.
const (
	branchID    = "1a6b8d1e-6c2a-4a55-8f7e-3c1d2b4a5e01"
	vipPathID   = "1a6b8d1e-6c2a-4a55-8f7e-3c1d2b4a5e02"
	otherwiseID = "1a6b8d1e-6c2a-4a55-8f7e-3c1d2b4a5e03"
	vipStepID   = "1a6b8d1e-6c2a-4a55-8f7e-3c1d2b4a5e04"
	nestedID    = "1a6b8d1e-6c2a-4a55-8f7e-3c1d2b4a5e05"
)

// vipCondition is a Branch path's Condition that tests the Tag "vip".
const vipCondition = `{"kind":"group","combinator":"and","entries":[{"kind":"tag","name":"vip","operator":"has"}]}`

// saveBranch creates an Automation whose root path holds the Branch written as doc, then a wait.
func saveBranch(t *testing.T, c *externalapi.Client, doc string) (*externalapi.AutomationResource, error) {
	t.Helper()
	return c.AutomationsCreate(t.Context(), &externalapi.CreateAutomationInput{
		Name: "Branches", Trigger: testhelper.ExternalTrigger("signup"),
		Steps: externalapi.NewOptAutomationPathInput(testhelper.ExternalPath(
			testhelper.ExternalBranch(t, doc),
			externalapi.WaitStepInput{Seconds: 60},
		)),
	})
}

// A Branch is kept as written (ADR 0124): its paths in order with their ids, Conditions and Steps,
// a Branch nested in a path, its otherwise; the Step after it stays after it.
func TestExternalAutomationKeepsABranchesTree(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.ExternalScoped(t, stepAuthorScopes...)

	created, err := saveBranch(t, c, `{"id":"`+branchID+`","type":"branch","paths":[
		{"id":"`+vipPathID+`","condition":`+vipCondition+`,"steps":[
			{"id":"`+vipStepID+`","type":"apply_tag","tag":"thanked"},
			{"id":"`+nestedID+`","type":"branch","paths":[{"condition":`+vipCondition+`,"steps":[]}],"otherwise":{"steps":[]}}
		]}],
		"otherwise":{"id":"`+otherwiseID+`","steps":[{"type":"wait","seconds":5}]}}`)
	require.NoError(t, err)
	got, err := c.AutomationsGet(t.Context(), externalapi.AutomationsGetParams{ID: created.ID})
	require.NoError(t, err)

	steps := got.Draft.Value.Steps.Steps
	require.Len(t, steps, 2)
	require.Equal(t, externalapi.BranchStepAutomationStepSum, steps[0].OneOf.Type)
	b := steps[0].OneOf.BranchStep
	assert.EqualValues(t, branchID, b.ID)
	require.Len(t, b.Paths, 1)
	assert.EqualValues(t, vipPathID, b.Paths[0].ID)
	cond, err := b.Paths[0].Condition.MarshalJSON()
	require.NoError(t, err)
	assert.JSONEq(t, vipCondition, string(cond))
	require.Len(t, b.Paths[0].Steps, 2)
	assert.EqualValues(t, vipStepID, b.Paths[0].Steps[0].OneOf.ApplyTagStep.ID)
	nested := b.Paths[0].Steps[1].OneOf
	require.Equal(t, externalapi.BranchStepPathStepSum, nested.Type)
	assert.NotEmpty(t, nested.BranchStep.Paths[0].ID, "a nested path is given an id")
	assert.NotEmpty(t, nested.BranchStep.Otherwise.ID)
	assert.EqualValues(t, otherwiseID, b.Otherwise.ID)
	assert.Len(t, b.Otherwise.Steps, 1)
	assert.False(t, b.Unknown.Set, "a Branch saved without an unknown path has none")
	assert.Equal(t, externalapi.WaitStepAutomationStepSum, steps[1].OneOf.Type)
}

// refusedNaming asserts err is the refusal of a save that names the Step by its id and kind.
func refusedNaming(t *testing.T, err error, id, kind string) string {
	t.Helper()
	refused := testhelper.ExternalRefusal(t, err, http.StatusUnprocessableEntity)
	detail := refused.Detail.Or("")
	assert.Contains(t, detail, id)
	assert.Contains(t, detail, "("+kind+")")
	return detail
}

// A Branch needs a path besides otherwise: one without is refused, naming it.
func TestExternalAutomationRefusesABranchWithOnlyOtherwise(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.ExternalScoped(t, stepAuthorScopes...)

	_, err := saveBranch(t, c, `{"id":"`+branchID+`","type":"branch","paths":[],"otherwise":{"steps":[]}}`)

	refusedNaming(t, err, branchID, "branch")
}

// Each path of a Branch has a Condition that tests something: an empty one is refused, naming the
// Branch.
func TestExternalAutomationRefusesABranchPathWithoutACondition(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.ExternalScoped(t, stepAuthorScopes...)

	_, err := saveBranch(t, c, `{"id":"`+branchID+`","type":"branch","paths":[
		{"condition":{"kind":"group","combinator":"and","entries":[]},"steps":[]}],"otherwise":{"steps":[]}}`)

	refusedNaming(t, err, branchID, "branch")
}

// A Branch path's Condition is checked as a Rule's: one naming a Custom field the Workspace lacks
// is refused, naming the Branch.
func TestExternalAutomationRefusesABranchConditionOnAnUnknownCustomField(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.ExternalScoped(t, stepAuthorScopes...)

	_, err := saveBranch(t, c, `{"id":"`+branchID+`","type":"branch","paths":[
		{"condition":{"kind":"group","combinator":"and","entries":[
			{"kind":"customField","key":"no_such_field","test":{"kind":"presence","operator":"set"}}]},"steps":[]}],
		"otherwise":{"steps":[]}}`)

	detail := refusedNaming(t, err, branchID, "branch")
	assert.Contains(t, detail, "no_such_field")
}

// A version holds at most 100 Steps over all its paths: the 101st, inside a Branch's path, is
// refused by its id.
func TestExternalAutomationRefusesTheHundredAndFirstStepOverAllPaths(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.ExternalScoped(t, stepAuthorScopes...)
	waits := make([]string, 98) // with the Branch and the wait after it, 100 Steps
	for i := range waits {
		waits[i] = `{"type":"wait","seconds":1}`
	}
	last := "1a6b8d1e-6c2a-4a55-8f7e-3c1d2b4a5e99"
	onlyBranch := func(steps []string) error {
		_, err := c.AutomationsCreate(t.Context(), &externalapi.CreateAutomationInput{
			Name: "Long", Trigger: testhelper.ExternalTrigger("signup"),
			Steps: externalapi.NewOptAutomationPathInput(testhelper.ExternalPath(testhelper.ExternalBranch(t,
				`{"type":"branch","paths":[{"condition":`+vipCondition+`,"steps":[`+strings.Join(steps, ",")+`]}],"otherwise":{"steps":[]}}`))),
		})
		return err
	}

	_, fits := saveBranch(t, c, `{"type":"branch","paths":[{"condition":`+vipCondition+`,"steps":[`+strings.Join(waits, ",")+`]}],"otherwise":{"steps":[]}}`)
	err := onlyBranch(append(waits, `{"type":"wait","seconds":1}`, `{"id":"`+last+`","type":"wait","seconds":1}`))

	require.NoError(t, fits)
	detail := refusedNaming(t, err, last, "wait")
	assert.Contains(t, detail, "100")
}

// A fork has at most 10 paths, otherwise counted: an eleventh refuses the save, naming the Branch.
func TestExternalAutomationRefusesABranchOfElevenPaths(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.ExternalScoped(t, stepAuthorScopes...)
	path := `{"condition":` + vipCondition + `,"steps":[]}`
	nine := strings.Repeat(path+",", 8) + path
	ten := nine + "," + path

	_, fits := saveBranch(t, c, fmt.Sprintf(`{"type":"branch","paths":[%s],"otherwise":{"steps":[]}}`, nine))
	_, err := saveBranch(t, c, fmt.Sprintf(`{"id":"%s","type":"branch","paths":[%s],"otherwise":{"steps":[]}}`, branchID, ten))

	require.NoError(t, fits)
	refusedNaming(t, err, branchID, "branch")
}
