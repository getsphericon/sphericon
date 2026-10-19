package mcpserver_test

import (
	"encoding/json"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/internal/brandkit"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/templates"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

func decode(t *testing.T, raw string) map[string]any {
	t.Helper()
	var out map[string]any
	require.NoError(t, json.Unmarshal([]byte(raw), &out))
	return out
}

func TestMCPBrandGetReturnsTheBrandKitAndWrapsItsTextAsUntrusted(t *testing.T) {
	env := testhelper.Setup(t)
	s := env.MCPClient(t, env.ScopedBearerFor(t, fixtures.InitechID, "templates:read"))

	res := call(t, s, "brand_get", map[string]any{})
	require.False(t, res.IsError, text(t, res))
	kit := decode(t, text(t, res))

	assert.Equal(t, true, kit["saved"])
	assert.Equal(t, "#0a7d5a", kit["accentColor"], "colors are the Workspace's values, checked on save")
	assert.EqualValues(t, 8, kit["buttonRadius"])
	// What a person of the Workspace wrote is data, never instructions.
	assert.Equal(t, map[string]any{"untrusted_data": "Georgia, serif"}, kit["headingFont"])
	assert.Equal(t, map[string]any{"untrusted_data": "https://cdn.initech.test/logo.png"}, kit["logoUrl"])
	assert.Equal(t, map[string]any{"untrusted_data": "Initech, 4120 Freidrich Lane"}, kit["footerText"])
}

func TestMCPBrandGetNeedsTheTemplatesReadScope(t *testing.T) {
	env := testhelper.Setup(t)
	s := env.MCPClient(t, env.ScopedBearer(t, "contacts:read"))
	assert.True(t, call(t, s, "brand_get", map[string]any{}).IsError)
	assert.True(t, call(t, s, "email_schema", map[string]any{}).IsError)
}

// The schema is what an agent writes a document from: a document built from its example is
// accepted, and the rules it names are the ones the server refuses by.
func TestMCPEmailSchemaDescribesTheDocumentAnAgentMayWrite(t *testing.T) {
	env := testhelper.Setup(t)
	s := env.MCPClient(t, env.ScopedBearer(t, "templates:read", "templates:write"))

	res := call(t, s, "email_schema", map[string]any{})
	require.False(t, res.IsError, text(t, res))
	schema := decode(t, text(t, res))

	nodes := schema["nodes"].(map[string]any)
	heading := nodes["heading"].(map[string]any)["attrs"].(map[string]any)["level"].(map[string]any)
	assert.Equal(t, "integer", heading["type"])
	assert.Contains(t, heading["rule"], "1 to 6")
	assert.Contains(t, schema["marks"], "link")
	assert.NotEmpty(t, schema["editorVersion"])
	assert.NotEmpty(t, schema["rules"])

	created := call(t, s, "templates_create", map[string]any{"name": "From the example", "doc": schema["example"]})
	require.False(t, created.IsError, text(t, created))
	assert.NotEmpty(t, decode(t, text(t, created))["body"], "the server rendered the body")
}

// An agent copies a starter Template: it reads one (document included) and writes a new Template
// with that document, edited. The starter is not changed.
func TestMCPAgentCopiesAStarterTemplateAndEditsItsDocument(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := t.Context()
	starters, err := brandkit.New(env.Bus, templates.New()).RecreateStarterTemplates(ctx, env.DB.Scoped(fixtures.InitechID))
	require.NoError(t, err)
	starter := starters[0]
	s := env.MCPClient(t, env.ScopedBearerFor(t, fixtures.InitechID, "templates:read", "templates:write"))

	read := call(t, s, "templates_get", map[string]any{"id": strconv.FormatInt(starter.ID, 10)})
	require.False(t, read.IsError, text(t, read))
	got := decode(t, text(t, read))
	require.NotNil(t, got["doc"], "a read returns the document")

	// The edit: the copy's document is the starter's with its heading's words changed.
	doc := got["doc"].(map[string]any)
	replaced := replaceText(doc, "Welcome", "Hello there")
	require.True(t, replaced, "the starter has a heading to edit")

	lintRes := call(t, s, "email_lint", map[string]any{"subject": got["subject"], "doc": doc})
	require.False(t, lintRes.IsError, text(t, lintRes))
	var lintOut lintResult
	require.NoError(t, json.Unmarshal([]byte(text(t, lintRes)), &lintOut))
	assert.Empty(t, lintOut.Errors)

	copied := call(t, s, "templates_create", map[string]any{"name": "My welcome", "subject": got["subject"], "doc": doc})
	require.False(t, copied.IsError, text(t, copied))
	out := decode(t, text(t, copied))
	assert.Contains(t, out["body"], "Hello there")
	assert.NotContains(t, out["body"], "Welcome")

	assert.Contains(t, env.DB.EmailTemplate.GetX(ctx, starter.ID).Body, "Welcome", "the starter is untouched")
}

// A document an agent writes under a prompt injection from contact data is refused with the
// reason, never repaired, and nothing is stored.
func TestMCPInvalidAndHostileDocumentsAreRefusedAndStoreNothing(t *testing.T) {
	env := testhelper.Setup(t)
	s := env.MCPClient(t, env.ScopedBearer(t, "templates:read", "templates:write", "broadcasts:write"))
	before := env.DB.EmailTemplate.Query().CountX(t.Context())

	hostile := map[string]any{"type": "doc", "content": []any{map[string]any{
		"type": "paragraph",
		"content": []any{map[string]any{
			"type": "text", "text": injection,
			"marks": []any{map[string]any{"type": "link", "attrs": map[string]any{"href": "javascript:alert(1)"}}},
		}},
	}}}
	res := call(t, s, "templates_create", map[string]any{"name": "Hostile", "doc": hostile})
	require.True(t, res.IsError)
	assert.Contains(t, text(t, res), "422")
	assert.Contains(t, text(t, res), "absolute URL")

	res = call(t, s, "broadcasts_create", map[string]any{"name": "Hostile", "doc": map[string]any{"type": "nope"}})
	require.True(t, res.IsError)
	assert.Contains(t, text(t, res), "422")

	res = call(t, s, "templates_create", map[string]any{"name": "Both", "body": "<p>x</p>", "doc": hostile})
	require.True(t, res.IsError)
	assert.Contains(t, text(t, res), "not both")

	assert.Equal(t, before, env.DB.EmailTemplate.Query().CountX(t.Context()), "nothing is stored")
}

// replaceText swaps old for new in the first text node that contains it, depth first.
func replaceText(node map[string]any, old, replacement string) bool {
	if s, ok := node["text"].(string); ok && len(old) > 0 {
		for i := 0; i+len(old) <= len(s); i++ {
			if s[i:i+len(old)] == old {
				node["text"] = s[:i] + replacement + s[i+len(old):]
				return true
			}
		}
	}
	children, _ := node["content"].([]any)
	for _, c := range children {
		if child, ok := c.(map[string]any); ok && replaceText(child, old, replacement) {
			return true
		}
	}
	return false
}
