package mcpserver_test

import (
	"encoding/json"
	"testing"

	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/emailtemplate"
	"github.com/getsphericon/sphericon/internal/domain/content/brandkits"
	"github.com/getsphericon/sphericon/internal/domain/identity/apitokens"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/kernel/emailrender"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

type lintResult struct {
	Errors   []map[string]any `json:"errors"`
	Warnings []map[string]any `json:"warnings"`
}

func lint(t *testing.T, env *testhelper.TestEnv, args map[string]any, scopes ...apitokens.Scope) lintResult {
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
		"body":    `<p style="color:#cc0000;font-family:Georgia, serif">Sale</p><img src="https://cdn.acme.co/a.png">`,
	}, "templates:read")

	assert.Empty(t, got.Errors)
	// Acme's saved Brand kit is Arial with #1a73e8, #202124, white.
	assert.ElementsMatch(t, []string{
		string(emailrender.CodeMissingAlt), string(emailrender.CodeOffBrandColor),
		string(emailrender.CodeOffBrandFont),
	}, codes(got.Warnings))
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
	s := env.MCPClient(t, env.ScopedBearer(t, apitokens.ScopeContactsRead))
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
	s := env.MCPClient(t, env.ScopedBearerFor(t, fixtures.VandelayID, apitokens.ScopeTemplatesRead))
	res := call(t, s, "email_lint", map[string]any{"body": `<p style="color:#cc0000;font-family:Georgia">x</p>`})
	require.False(t, res.IsError, text(t, res))
	var got lintResult
	require.NoError(t, json.Unmarshal([]byte(text(t, res)), &got))
	assert.Empty(t, got.Warnings)
}

// A link or an image left at a name reserved for examples is a placeholder (ADR 0188); a host a
// merge tag gives is decided per recipient and is not checked.
func TestMCPEmailLintWarnsOnAReservedHost(t *testing.T) {
	env := testhelper.Setup(t)

	got := lint(t, env, map[string]any{
		"body": `<p><a href="https://example.com/x">Shop</a> <a href="https://shop.acme.test">Cart</a></p>` +
			`<img src="https://cdn.example.org/a.png" alt="">`,
	}, "templates:read")

	assert.Empty(t, got.Errors)
	assert.Equal(t, []string{"reserved_host", "reserved_host", "reserved_host"}, codes(got.Warnings))
	assert.Equal(t, map[string]any{"untrusted_data": "https://cdn.example.org/a.png"}, got.Warnings[2]["detail"],
		"the URL quotes the message, so it is wrapped")
}

func TestMCPEmailLintLeavesAHostThatIsNotReservedAlone(t *testing.T) {
	env := testhelper.Setup(t)

	got := lint(t, env, map[string]any{
		"body": `<p><a href="https://examples.com">Shop</a> <a href="{{ url }}">Yours</a> <a href="https://{{ host }}/x">Go</a></p>`,
	}, "templates:read")

	assert.Empty(t, got.Errors)
	assert.Empty(t, got.Warnings)
}

// startersOfAFirstSave saves Vandelay's first Brand kit, the defaults with the given website, and
// returns the starter Templates the save made.
func startersOfAFirstSave(t *testing.T, env *testhelper.TestEnv, website string) []*ent.EmailTemplate {
	t.Helper()
	ctx := t.Context()
	s := env.DB.Scoped(fixtures.VandelayID)
	lastBefore := s.EmailTemplate().Query().Order(ent.Desc(emailtemplate.FieldID)).FirstX(ctx).ID
	kit := brandkits.Defaults()
	_, err := testhelper.Module[*brandkits.Module](env).Save(ctx, s, brandkits.Input{
		Revision: kit.Revision, AccentColor: kit.AccentColor, TextColor: kit.TextColor, BackgroundColor: kit.BackgroundColor,
		HeadingFont: kit.HeadingFont, BodyFont: kit.BodyFont, WebsiteURL: website,
		ButtonTextColor: kit.ButtonTextColor, ButtonRadius: kit.ButtonRadius, FooterText: "Vandelay Industries",
	})
	require.NoError(t, err)
	starters := s.EmailTemplate().Query().Where(emailtemplate.IDGT(lastBefore)).AllX(ctx)
	require.Len(t, starters, 4)
	return starters
}

// lintVandelayStarter lints a starter Template as an agent of Vandelay would write it: its document
// rendered by the editor's own export (the test process's save paths store a stand-in body).
func lintVandelayStarter(t *testing.T, env *testhelper.TestEnv, tpl *ent.EmailTemplate) lintResult {
	t.Helper()
	subject := tpl.Subject
	rendered, err := emailrender.FromDocument(t.Context(), testhelper.RealRenderer(), &subject, tpl.Doc)
	require.NoError(t, err)
	client := env.MCPClient(t, env.ScopedBearerFor(t, fixtures.VandelayID, apitokens.ScopeTemplatesRead))
	res := call(t, client, "email_lint", map[string]any{"subject": subject, "body": rendered.Body})
	require.False(t, res.IsError, text(t, res))
	var got lintResult
	require.NoError(t, json.Unmarshal([]byte(text(t, res)), &got))
	return got
}

func oneButtonStarter(t *testing.T, starters []*ent.EmailTemplate) *ent.EmailTemplate {
	t.Helper()
	buttons := lo.Filter(starters, func(tpl *ent.EmailTemplate, _ int) bool { return tpl.Name == "One button" })
	require.Len(t, buttons, 1)
	return buttons[0]
}

// A starter Template made from a Brand kit with a website links its button there and has nothing
// to warn about (ADR 0188).
func TestMCPEmailLintFindsNothingInAStarterMadeWithAWebsite(t *testing.T) {
	env := testhelper.Setup(t)
	starters := startersOfAFirstSave(t, env, "https://vandelay.industries")

	for _, tpl := range starters {
		t.Run(tpl.Name, func(t *testing.T) {
			got := lintVandelayStarter(t, env, tpl)

			assert.Empty(t, got.Errors)
			assert.Empty(t, got.Warnings)
		})
	}
	assert.Contains(t, string(oneButtonStarter(t, starters).Doc), `"href": "https://vandelay.industries"`)
}

// Without a website the starters' button keeps the placeholder, which is warned about.
func TestMCPEmailLintWarnsOnTheButtonOfAStarterMadeWithoutAWebsite(t *testing.T) {
	env := testhelper.Setup(t)
	button := oneButtonStarter(t, startersOfAFirstSave(t, env, ""))

	got := lintVandelayStarter(t, env, button)

	assert.Equal(t, []string{"reserved_host"}, codes(got.Warnings))
}
