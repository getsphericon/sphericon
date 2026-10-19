package mcpserver_test

import (
	"regexp"
	"slices"
	"testing"

	"github.com/getsphericon/sphericon/internal/testhelper"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Independent literal: the playbooks sphericon ships.
var shippedPlaybooks = []string{"welcome", "win-back", "list-hygiene"}

func listedPrompts(t *testing.T, s *mcp.ClientSession) map[string]*mcp.Prompt {
	t.Helper()
	list, err := s.ListPrompts(t.Context(), nil)
	require.NoError(t, err)
	got := map[string]*mcp.Prompt{}
	for _, p := range list.Prompts {
		got[p.Name] = p
	}
	return got
}

func promptText(t *testing.T, s *mcp.ClientSession, name string) string {
	t.Helper()
	res, err := s.GetPrompt(t.Context(), &mcp.GetPromptParams{Name: name})
	require.NoError(t, err)
	require.Len(t, res.Messages, 1)
	tc, ok := res.Messages[0].Content.(*mcp.TextContent)
	require.Truef(t, ok, "got %T", res.Messages[0].Content)
	return tc.Text
}

func TestMCPShippedPlaybooksAreListedAndRetrievable(t *testing.T) {
	env := testhelper.Setup(t)
	s := env.MCPClient(t, env.ScopedBearer(t, "contacts:read"))

	prompts := listedPrompts(t, s)
	for _, name := range shippedPlaybooks {
		p, ok := prompts[name]
		require.Truef(t, ok, "prompt %q is not listed", name)
		assert.NotEmpty(t, p.Description)
		assert.NotEmpty(t, promptText(t, s, name))
	}
	assert.Len(t, prompts, len(shippedPlaybooks))

	_, err := s.GetPrompt(t.Context(), &mcp.GetPromptParams{Name: "nonexistent"})
	assert.Error(t, err)
}

// A playbook names a tool as `tool_name()`. Every such reference must exist in the
// tool surface projected from the contract, and none may be a send-class tool: the
// agent authors, a human reviews and sends.
func TestMCPPlaybooksOnlyReferenceAuthoringTools(t *testing.T) {
	env := testhelper.Setup(t)
	// mcp:send lists the whole generated surface, send-class tools included.
	surface := listedTools(t, env.MCPClient(t, env.ScopedBearer(t, "mcp:send")))
	for _, name := range sendTools {
		require.Contains(t, surface, name)
	}
	s := env.MCPClient(t, env.ScopedBearer(t, "contacts:read"))
	ref := regexp.MustCompile("`([a-z][a-z0-9_]*)\\(\\)`")

	for _, name := range shippedPlaybooks {
		body := promptText(t, s, name)
		refs := ref.FindAllStringSubmatch(body, -1)
		assert.NotEmptyf(t, refs, "%s references no tools", name)
		for _, m := range refs {
			assert.Containsf(t, surface, m[1], "%s references unknown tool %s", name, m[1])
			assert.Falsef(t, slices.Contains(sendTools, m[1]), "%s references send-class tool %s", name, m[1])
		}
		for _, send := range sendTools {
			assert.NotRegexpf(t, `\b`+send+`\b`, body, "%s mentions send-class tool %s", name, send)
		}
	}
}

// The playbooks that write mail tell an agent that the editor's document is the format and name
// where the Brand kit, the schema and the lint are (ADR 0040).
func TestMCPPlaybooksThatWriteMailNameTheEditorTools(t *testing.T) {
	env := testhelper.Setup(t)
	s := env.MCPClient(t, env.ScopedBearer(t, "contacts:read"))

	for _, name := range []string{"welcome", "win-back"} {
		body := promptText(t, s, name)
		for _, tool := range []string{"brand_get", "email_schema", "email_lint", "templates_get"} {
			assert.Containsf(t, body, "`"+tool+"()`", "%s names %s", name, tool)
		}
		assert.Containsf(t, body, "editor's document", "%s says the document is the format", name)
		assert.NotContainsf(t, body, "HTML with merge tags", "%s no longer asks for an HTML body", name)
	}
}
