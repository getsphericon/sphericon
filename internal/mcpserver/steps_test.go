package mcpserver_test

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/internal/domain/identity/apitokens"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// readSteps is a version's steps as an agent reads them back: the root path and its Steps.
type readSteps struct {
	ID    string           `json:"id"`
	Steps []map[string]any `json:"steps"`
}

// An agent reads a version back as its Steps in order (ADR 0122): the root path, each Step typed by
// `type` with only its kind's fields and an id, so it can show the flow as a numbered list before
// asking for a Publish.
func TestMCPReadsAnAutomationVersionBackAsItsStepsInOrder(t *testing.T) {
	env := testhelper.Setup(t)
	s := env.MCPClient(t, env.ScopedBearer(t, apitokens.ScopeAutomationsRead, apitokens.ScopeAutomationsWrite))
	var steps map[string]any
	require.NoError(t, json.Unmarshal([]byte(`{"steps":[
		{"type":"email","subject":"Welcome","body":"<p>Hi</p>"},
		{"type":"wait","seconds":3600},
		{"type":"apply_tag","tag":"welcomed"},
		{"type":"end"}
	]}`), &steps))
	created := call(t, s, "automations_create", map[string]any{"name": "Agent's welcome", "trigger": map[string]any{"kind": "event", "action": "signup"}, "steps": steps})
	require.False(t, created.IsError, text(t, created))
	var made struct {
		ID string `json:"id"`
	}
	require.NoError(t, json.Unmarshal([]byte(text(t, created)), &made))

	got := call(t, s, "automations_get", map[string]any{"id": made.ID})

	require.False(t, got.IsError, text(t, got))
	var read struct {
		Draft struct {
			Steps readSteps `json:"steps"`
		} `json:"draft"`
	}
	require.NoError(t, json.Unmarshal([]byte(text(t, got)), &read))
	path := read.Draft.Steps
	assert.NotEmpty(t, path.ID, "the root path has an id")
	require.Len(t, path.Steps, 4)
	kinds := make([]any, len(path.Steps))
	for i, st := range path.Steps {
		kinds[i] = st["type"]
		assert.NotEmpty(t, st["id"], "step %d has an id", i+1)
	}
	assert.Equal(t, []any{"email", "wait", "apply_tag", "end"}, kinds)
	assert.Equal(t, "Welcome", path.Steps[0]["subject"])
	assert.InDelta(t, 3600, path.Steps[1]["seconds"], 0)
	assert.NotContains(t, path.Steps[1], "subject", "a Step carries only its kind's fields")
	assert.Equal(t, "welcomed", path.Steps[2]["tag"])
}

// The tool that writes an Automation carries the Step union in `$defs`: its path's Steps refer to
// the union, whose discriminator maps every kind to its closed model, and every reference
// resolves.
func TestMCPToolCarriesTheStepUnionInDefs(t *testing.T) {
	env := testhelper.Setup(t)
	s := env.MCPClient(t, env.ScopedBearer(t, apitokens.ScopeAutomationsWrite))
	tools := listedTools(t, s)
	require.Contains(t, tools, "automations_create")
	raw, err := json.Marshal(tools["automations_create"].InputSchema)
	require.NoError(t, err)
	var schema struct {
		Properties map[string]map[string]any `json:"properties"`
		Defs       map[string]map[string]any `json:"$defs"`
	}
	require.NoError(t, json.Unmarshal(raw, &schema))

	steps := schema.Defs["AutomationPathInput"]["properties"].(map[string]any)["steps"].(map[string]any)
	mapping := schema.Defs["AutomationStepInput"]["discriminator"].(map[string]any)["mapping"].(map[string]any)

	assert.Equal(t, "#/$defs/AutomationPathInput", refOf(schema.Properties["steps"]))
	assert.Equal(t, "#/$defs/AutomationStepInput", steps["items"].(map[string]any)["$ref"])
	assert.Equal(t, "#/$defs/WaitStepInput", mapping["wait"])
	assert.Len(t, mapping, 9, "every kind of Step")
	assert.Equal(t, "#/$defs/EndStepInput", mapping["end"])
	assert.Equal(t, "#/$defs/BranchStepInput", mapping["branch"])
	assert.Empty(t, unresolvedRefs(raw, schema.Defs))
}
