package mcpserver_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/internal/domain/identity/apitokens"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// SPIKE (#1674): a tool whose body takes a place's Condition carries the recursive tree in `$defs`,
// every reference in it resolves there, and a call with a nested tree reaches /api and comes back.
func TestMCPToolCarriesThePlacesRecursiveConditionInDefs(t *testing.T) {
	env := testhelper.Setup(t)
	s := env.MCPClient(t, env.ScopedBearer(t, apitokens.ScopeSegmentsWrite))
	tool := listedTools(t, s)["condition_spike_rule"]
	require.NotNil(t, tool)
	raw, err := json.Marshal(tool.InputSchema)
	require.NoError(t, err)
	var schema struct {
		Properties map[string]map[string]any `json:"properties"`
		Defs       map[string]map[string]any `json:"$defs"`
	}
	require.NoError(t, json.Unmarshal(raw, &schema))

	assert.Equal(t, "#/$defs/RuleConditionInput", schema.Properties["condition"]["$ref"])
	entries := schema.Defs["RuleConditionInput"]["properties"].(map[string]any)["entries"].(map[string]any)
	assert.Equal(t, "#/$defs/RuleConditionNodeInput", entries["items"].(map[string]any)["$ref"])
	node := schema.Defs["RuleConditionNodeInput"]
	assert.Equal(t, "#/$defs/RuleConditionInput", node["discriminator"].(map[string]any)["mapping"].(map[string]any)["group"],
		"the group refers back to its own place's tree")
	for _, ref := range refsIn(raw) {
		name, local := strings.CutPrefix(ref, "#/$defs/")
		require.Truef(t, local, "%s is not a local ref", ref)
		assert.Containsf(t, schema.Defs, name, "%s resolves in $defs", ref)
	}

	const condition = `{"kind":"group","combinator":"and","entries":[{"kind":"group","combinator":"or","entries":[
		{"kind":"property","of":{"kind":"trigger"},"key":"cart.total","test":{"kind":"number","operator":"gt","value":100}}]}]}`
	var arg map[string]any
	require.NoError(t, json.Unmarshal([]byte(condition), &arg))
	res := call(t, s, "condition_spike_rule", map[string]any{"condition": arg})
	require.False(t, res.IsError, text(t, res))
	assert.Contains(t, text(t, res), `"key":"cart.total"`)
}

// refsIn is every `$ref` and discriminator mapping target of a schema document.
func refsIn(doc []byte) []string {
	var v any
	if json.Unmarshal(doc, &v) != nil {
		return nil
	}
	var out []string
	var walk func(any, bool)
	walk = func(n any, inMapping bool) {
		switch x := n.(type) {
		case map[string]any:
			for k, val := range x {
				if s, ok := val.(string); ok && (k == "$ref" || inMapping) {
					out = append(out, s)
					continue
				}
				walk(val, k == "mapping")
			}
		case []any:
			for _, val := range x {
				walk(val, false)
			}
		}
	}
	walk(v, false)
	return out
}
