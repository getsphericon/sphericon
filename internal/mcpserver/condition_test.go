package mcpserver_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/internal/domain/identity/apitokens"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// A tool whose body takes a place's Condition carries the place's recursive tree in `$defs` (ADR
// 0181): its group refers back to its own place, and every reference, a union's mapping included,
// resolves there.
func TestMCPToolCarriesThePlacesConditionInDefs(t *testing.T) {
	env := testhelper.Setup(t)
	s := env.MCPClient(t, env.ScopedBearer(t, apitokens.ScopeSegmentsWrite, apitokens.ScopeAutomationsWrite))
	tools := listedTools(t, s)

	for tool, place := range map[string]string{"segments_create": "SegmentCondition", "rules_create": "RuleCondition"} {
		require.Contains(t, tools, tool)
		raw, err := json.Marshal(tools[tool].InputSchema)
		require.NoError(t, err)
		var schema struct {
			Properties map[string]map[string]any `json:"properties"`
			Defs       map[string]map[string]any `json:"$defs"`
		}
		require.NoError(t, json.Unmarshal(raw, &schema))
		entries := schema.Defs[place+"Input"]["properties"].(map[string]any)["entries"].(map[string]any)
		mapping := schema.Defs[place+"NodeInput"]["discriminator"].(map[string]any)["mapping"].(map[string]any)
		unresolved := unresolvedRefs(raw, schema.Defs)

		assert.Equal(t, "#/$defs/"+place+"Input", refOf(schema.Properties["condition"]), tool)
		assert.Equal(t, "#/$defs/"+place+"NodeInput", entries["items"].(map[string]any)["$ref"], tool)
		assert.Equal(t, "#/$defs/"+place+"Input", mapping["group"], "%s: the group refers back to its own place", tool)
		assert.Empty(t, unresolved, tool)
	}
	assert.NotContains(t, listedDefs(t, tools["segments_create"]), "RulePropertyLeafInput", "a Segment's tree holds no property test")
	assert.Contains(t, listedDefs(t, tools["rules_create"]), "RulePropertyLeafInput")
}

// A Condition an agent sends that its place does not admit is refused, naming the offending
// leaf's path.
func TestMCPRefusesAConditionLeafByItsPath(t *testing.T) {
	env := testhelper.Setup(t)
	s := env.MCPClient(t, env.ScopedBearer(t, apitokens.ScopeSegmentsWrite))
	var condition map[string]any
	require.NoError(t, json.Unmarshal([]byte(`{"kind":"group","combinator":"and","entries":[
		{"kind":"tag","name":"vip","operator":"has"},
		{"kind":"group","combinator":"or","entries":[
			{"kind":"customField","key":"courses_completed","test":{"kind":"number","operator":"contains","value":1}}
		]}
	]}`), &condition))

	res := call(t, s, "segments_create", map[string]any{"name": "Misplaced", "condition": condition})

	assert.True(t, res.IsError)
	assert.Contains(t, text(t, res), "condition.entries.1.entries.0.test.operator")
}

// refOf is the ref a property's schema names: its own, or the one of its allOf, which a property
// with a description of its own wraps the ref in.
func refOf(property map[string]any) any {
	if ref, ok := property["$ref"]; ok {
		return ref
	}
	all, _ := property["allOf"].([]any)
	if len(all) != 1 {
		return nil
	}
	only, _ := all[0].(map[string]any)
	return only["$ref"]
}

// listedDefs is the names of the `$defs` of a tool's input schema.
func listedDefs(t *testing.T, tool *mcp.Tool) []string {
	t.Helper()
	raw, err := json.Marshal(tool.InputSchema)
	require.NoError(t, err)
	var schema struct {
		Defs map[string]any `json:"$defs"`
	}
	require.NoError(t, json.Unmarshal(raw, &schema))
	names := make([]string, 0, len(schema.Defs))
	for name := range schema.Defs {
		names = append(names, name)
	}
	return names
}

// unresolvedRefs is every `$ref` and discriminator mapping target of a schema document that is not
// a ref to one of defs.
func unresolvedRefs(doc []byte, defs map[string]map[string]any) []string {
	var v any
	if json.Unmarshal(doc, &v) != nil {
		return []string{"<not JSON>"}
	}
	var out []string
	var walk func(n any, inMapping bool)
	walk = func(n any, inMapping bool) {
		switch x := n.(type) {
		case map[string]any:
			for k, val := range x {
				if ref, ok := val.(string); ok && (k == "$ref" || inMapping) {
					name, local := strings.CutPrefix(ref, "#/$defs/")
					if _, found := defs[name]; !local || !found {
						out = append(out, ref)
					}
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

// An agent writes a Trigger as an Event matcher and reads it back as it wrote it; a test of the
// Contact in its filter is refused, naming the leaf's path (ADR 0133, ADR 0181).
func TestMCPWritesTheTriggerAsAnEventMatcher(t *testing.T) {
	env := testhelper.Setup(t)
	s := env.MCPClient(t, env.ScopedBearer(t, apitokens.ScopeAutomationsRead, apitokens.ScopeAutomationsWrite))
	const trigger = `{"kind":"event","action":"order.completed","filter":{"kind":"group","combinator":"and","entries":[
		{"kind":"property","key":"status","test":{"kind":"text","operator":"eq","value":"paid"}}
	]}}`
	var matcher, misplaced map[string]any
	require.NoError(t, json.Unmarshal([]byte(trigger), &matcher))
	require.NoError(t, json.Unmarshal([]byte(`{"kind":"event","action":"order.completed","filter":{"kind":"group","combinator":"and","entries":[
		{"kind":"tag","name":"vip","operator":"has"}
	]}}`), &misplaced))

	created := call(t, s, "automations_create", map[string]any{"name": "Paid orders", "trigger": matcher})
	require.False(t, created.IsError, text(t, created))
	var made struct {
		ID    string `json:"id"`
		Draft struct {
			Trigger json.RawMessage `json:"trigger"`
		} `json:"draft"`
	}
	require.NoError(t, json.Unmarshal([]byte(text(t, created)), &made))
	refused := call(t, s, "automations_create", map[string]any{"name": "Misplaced", "trigger": misplaced})

	readBack := strings.TrimSuffix(trigger, "}") + `,"entryPolicy":"once","cooldown":3600}`
	assert.JSONEq(t, readBack, string(made.Draft.Trigger))
	assert.True(t, refused.IsError)
	assert.Contains(t, text(t, refused), "trigger.filter.entries.0")
}
