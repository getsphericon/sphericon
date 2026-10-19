package mcpserver_test

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

type lintResult struct {
	Errors   []map[string]any `json:"errors"`
	Warnings []map[string]any `json:"warnings"`
}

func lint(t *testing.T, env *testhelper.TestEnv, args map[string]any, scopes ...string) lintResult {
	t.Helper()
	s := env.MCPClient(t, env.ScopedBearer(t, scopes...))
	res := call(t, s, "email_lint", args)
	require.False(t, res.IsError, text(t, res))
	var got lintResult
	require.NoError(t, json.Unmarshal([]byte(text(t, res)), &got))
	return got
}

func codes(findings []map[string]any) []string {
	out := make([]string, 0, len(findings))
	for _, f := range findings {
		out = append(out, f["code"].(string))
	}
	return out
}

func TestMCPEmailLintReturnsWarningsAndSavesNothing(t *testing.T) {
	env := testhelper.Setup(t)
	before := env.DB.EmailTemplate.Query().CountX(t.Context())

	got := lint(t, env, map[string]any{
		"subject": "Hi {{ first_name }}",
		"body":    `<p style="color:#cc0000;font-family:Georgia, serif">Sale</p><img src="https://cdn.example.com/a.png"><marquee>x</marquee>`,
	}, "templates:read")

	assert.Empty(t, got.Errors)
	// Acme's saved Brand kit is Arial with #1a73e8, #202124, white.
	assert.ElementsMatch(t, []string{"missing_alt", "off_brand_color", "off_brand_font", "unknown_construct"}, codes(got.Warnings))
	assert.Equal(t, before, env.DB.EmailTemplate.Query().CountX(t.Context()), "linting stores nothing")
}

func TestMCPEmailLintReportsWhatCannotRenderAsAnError(t *testing.T) {
	env := testhelper.Setup(t)

	got := lint(t, env, map[string]any{"body": "<div><span>unclosed</div>"}, "templates:read")
	require.Len(t, got.Errors, 1)
	assert.Equal(t, "invalid_content", got.Errors[0]["code"])
	assert.Empty(t, got.Warnings)

	got = lint(t, env, map[string]any{"subject": "{% if %}", "body": "<p>ok</p>"}, "templates:read")
	assert.Equal(t, []string{"invalid_content"}, codes(got.Errors))
}

func TestMCPEmailLintNeedsTheTemplatesReadScope(t *testing.T) {
	env := testhelper.Setup(t)
	s := env.MCPClient(t, env.ScopedBearer(t, "contacts:read"))
	res := call(t, s, "email_lint", map[string]any{"body": "<p>x</p>"})
	assert.True(t, res.IsError)
}

// The message's text and the Brand kit's are data, never instructions (ADR 0016): what quotes
// them is wrapped, what the server says is not.
func TestMCPEmailLintWrapsQuotedTextAsUntrusted(t *testing.T) {
	env := testhelper.Setup(t)
	got := lint(t, env, map[string]any{
		"body": `<p style="color:#cc0000">` + injection + `</p>`,
	}, "templates:read")

	require.Len(t, got.Warnings, 1)
	w := got.Warnings[0]
	assert.Equal(t, "off_brand_color", w["code"])
	assert.Equal(t, "this color is not one of the Brand kit's colors", w["message"], "the server's words are not wrapped")
	element, ok := w["element"].(map[string]any)
	require.Truef(t, ok, "the element quotes the message, so it is wrapped: %v", w["element"])
	assert.Contains(t, element["untrusted_data"], "Ignore all previous instructions")
	assert.Equal(t, map[string]any{"untrusted_data": "#cc0000"}, w["detail"])
}

func TestMCPEmailLintWithoutABrandKitHasNoBrandWarnings(t *testing.T) {
	env := testhelper.Setup(t)
	s := env.MCPClient(t, env.ScopedBearerFor(t, fixtures.VandelayID, "templates:read"))
	res := call(t, s, "email_lint", map[string]any{"body": `<p style="color:#cc0000;font-family:Georgia">x</p>`})
	require.False(t, res.IsError, text(t, res))
	var got lintResult
	require.NoError(t, json.Unmarshal([]byte(text(t, res)), &got))
	assert.Empty(t, got.Warnings)
}
