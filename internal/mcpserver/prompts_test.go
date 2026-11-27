package mcpserver_test

import (
	"regexp"
	"slices"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/internal/domain/identity/apitokens"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// Independent literal: the prompts sphericon ships.
var shippedPrompts = []string{"welcome", "win-back", "list-hygiene", "set-up-sending-domain"}

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

func TestMCPShippedPromptsAreListedAndRetrievable(t *testing.T) {
	env := testhelper.Setup(t)
	s := env.MCPClient(t, env.ScopedBearer(t, apitokens.ScopeContactsRead))

	prompts := listedPrompts(t, s)
	for _, name := range shippedPrompts {
		p, ok := prompts[name]
		require.Truef(t, ok, "prompt %q is not listed", name)
		assert.NotEmpty(t, p.Description)
		assert.NotEmpty(t, promptText(t, s, name))
	}
	assert.Len(t, prompts, len(shippedPrompts))

	_, err := s.GetPrompt(t.Context(), &mcp.GetPromptParams{Name: "nonexistent"})
	assert.Error(t, err)
}

// A prompt names a tool as `tool_name()`. Every such reference must exist in the
// tool surface projected from the contract, and none may be a send-class tool: the
// agent authors, a human reviews and sends.
func TestMCPPromptsOnlyReferenceAuthoringTools(t *testing.T) {
	env := testhelper.Setup(t)
	// mcp:send lists the whole generated surface, send-class tools included.
	surface := listedTools(t, env.MCPClient(t, env.ScopedBearer(t, apitokens.ScopeMcpSend)))
	for _, name := range sendTools {
		require.Contains(t, surface, name)
	}
	s := env.MCPClient(t, env.ScopedBearer(t, apitokens.ScopeContactsRead))
	ref := regexp.MustCompile("`([a-z][a-z0-9_]*)\\(\\)`")

	for _, name := range shippedPrompts {
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

// whoami names the token, not the Workspace: every prompt starts by reading the Workspace and
// saying its name back to the human, so the agent never reports a Workspace it guessed.
func TestMCPPromptsStartByNamingTheWorkspace(t *testing.T) {
	env := testhelper.Setup(t)
	s := env.MCPClient(t, env.ScopedBearer(t, apitokens.ScopeContactsRead))
	step1 := regexp.MustCompile(`(?m)^1\. (.*)$`)

	for _, name := range shippedPrompts {
		body := promptText(t, s, name)
		step := step1.FindStringSubmatch(body)
		require.NotNilf(t, step, "%s has no step 1", name)
		assert.Containsf(t, step[1], "`workspace_get()`", "%s reads the Workspace in step 1", name)
		assert.Containsf(t, step[1], "Workspace's name", "%s says the Workspace's name back in step 1", name)
		assert.NotContainsf(t, step[1], "whoami", "%s no longer confirms the Workspace with whoami", name)
	}
}

// The prompts that write mail tell an agent that the editor's document is the format and name
// where the Brand kit, the schema and the lint are (ADR 0040).
func TestMCPPromptsThatWriteMailNameTheEditorTools(t *testing.T) {
	env := testhelper.Setup(t)
	s := env.MCPClient(t, env.ScopedBearer(t, apitokens.ScopeContactsRead))

	for _, name := range []string{"welcome", "win-back"} {
		body := promptText(t, s, name)
		for _, tool := range []string{"brand_get", "email_schema", "email_lint", "templates_get"} {
			assert.Containsf(t, body, "`"+tool+"()`", "%s names %s", name, tool)
		}
		assert.Containsf(t, body, "editor's document", "%s says the document is the format", name)
		assert.NotContainsf(t, body, "HTML with merge tags", "%s no longer asks for an HTML body", name)
	}
}

// The win-back prompt names the audience when it writes the Broadcast, and reads back the
// recipients the preview counts on the Broadcast's Channel before it hands over (ADR 0149).
func TestMCPWinBackPromptNamesTheAudienceAndCountsItsRecipients(t *testing.T) {
	env := testhelper.Setup(t)
	s := env.MCPClient(t, env.ScopedBearer(t, apitokens.ScopeContactsRead))

	body := promptText(t, s, "win-back")

	assert.Contains(t, body, "`segmentId`", "the draft is written with its audience")
	assert.Contains(t, body, "`channel`", "the preview is asked on the Broadcast's Channel")
	assert.Contains(t, body, "`recipients`", "the count read back is the recipients")
	assert.NotContains(t, body, "broadcasts_set_segment", "the audience is named at create, not after")
}

// The win-back prompt measures engagement by clicks or a product action, never by opens: a mail
// client's privacy proxy opens messages no one read (ADR 0123).
func TestMCPWinBackPromptMeasuresClicksNotOpens(t *testing.T) {
	env := testhelper.Setup(t)
	s := env.MCPClient(t, env.ScopedBearer(t, apitokens.ScopeContactsRead))

	body := promptText(t, s, "win-back")

	assert.Contains(t, body, "clicks or a product action", "engagement is clicks or a product action")
	assert.NotRegexp(t, `opens, clicks|opens or clicks`, body, "opens are not offered as a measure")
	assert.Contains(t, body, "Opens are not a reliable signal", "the prompt says why opens are left out")
}

// The set-up prompt proposes one subdomain per stream, so the Transactional stream's reputation is
// kept apart from the marketing one's, and verifies both.
func TestMCPSetUpSendingDomainPromptProposesASubdomainPerStream(t *testing.T) {
	env := testhelper.Setup(t)
	s := env.MCPClient(t, env.ScopedBearer(t, apitokens.ScopeContactsRead))

	body := promptText(t, s, "set-up-sending-domain")

	assert.Contains(t, body, "`mail.example.com`", "a subdomain for marketing mail")
	assert.Contains(t, body, "`notify.example.com`", "a subdomain for Transactional mail")
	assert.Contains(t, body, "verify both", "both subdomains are verified")
	assert.NotContains(t, body, "a subdomain such as `mail.example.com` is the usual choice",
		"no single subdomain for all mail")
}
