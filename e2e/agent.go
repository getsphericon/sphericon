//go:build e2e

package e2e

import (
	"encoding/json"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/require"

	externalapi "github.com/getsphericon/sphericon/gen/external"
	"github.com/getsphericon/sphericon/internal/apitokens"
	"github.com/getsphericon/sphericon/internal/brandkit"
	"github.com/getsphericon/sphericon/internal/cleanup"
	"github.com/getsphericon/sphericon/internal/i18n"
)

// SaveBrandKit is a person saving the Workspace's Brand kit in the app: the first save makes the
// starter Templates in its brand (ADR 0040).
func (w *Workspace) SaveBrandKit() {
	w.t.Helper()
	_, err := w.env.brandKit.Save(w.t.Context(), w.scoped, brandkit.Input{
		AccentColor:     "#0a7d5a",
		TextColor:       "#1d1d1f",
		BackgroundColor: "#fafafa",
		HeadingFont:     "Georgia, serif",
		BodyFont:        "Verdana, sans-serif",
		ButtonTextColor: "#ffffff",
		ButtonRadius:    8,
		FooterText:      "E2E Inc, 1 Test Street",
	})
	require.NoError(w.t, err, "save the Brand kit")
}

// StarterWelcomeName is the name of the welcome starter Template in the instance's locale.
func StarterWelcomeName() string { return i18n.T("brandkit.welcome.name", nil) }

// agentScopes are what an agent that only drafts holds: no send scope and no mcp:send.
var agentScopes = scopeStrings(
	externalapi.ApiTokenScopeTemplatesRead, externalapi.ApiTokenScopeTemplatesWrite,
	externalapi.ApiTokenScopeBroadcastsRead, externalapi.ApiTokenScopeBroadcastsWrite,
)

// Agent is an AI agent connected to the Workspace over MCP with a token that can draft but not
// send. Every step is a tool call, over the real listener.
type Agent struct {
	w       *Workspace
	session *mcp.ClientSession
}

// Agent connects an agent over MCP with its own authoring-only token.
func (w *Workspace) Agent() *Agent {
	w.t.Helper()
	ctx := w.t.Context()
	minted, err := apitokens.Mint(ctx, w.scoped, apitokens.Input{Name: "e2e agent", Scopes: agentScopes})
	require.NoError(w.t, err)

	transport := &mcp.StreamableClientTransport{
		Endpoint:             w.env.BaseURL + "/mcp",
		HTTPClient:           &http.Client{Transport: bearerTransport{token: minted.Value}},
		DisableStandaloneSSE: true,
		MaxRetries:           -1,
	}
	session, err := mcp.NewClient(&mcp.Implementation{Name: "e2e-agent", Version: "0"}, nil).Connect(ctx, transport, nil)
	require.NoError(w.t, err, "connect the agent over MCP")
	w.t.Cleanup(func() { cleanup.Report("close the MCP session", session.Close()) })
	return &Agent{w: w, session: session}
}

type bearerTransport struct{ token string }

func (b bearerTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	r = r.Clone(r.Context())
	r.Header.Set("Authorization", "Bearer "+b.token)
	return http.DefaultTransport.RoundTrip(r)
}

// Tools lists the tool names the agent is offered.
func (a *Agent) Tools() []string {
	a.w.t.Helper()
	list, err := a.session.ListTools(a.w.t.Context(), nil)
	require.NoError(a.w.t, err)
	names := make([]string, 0, len(list.Tools))
	for _, tool := range list.Tools {
		names = append(names, tool.Name)
	}
	return names
}

// call runs a tool and returns its result: the JSON object on success, the error text otherwise.
func (a *Agent) call(name string, args map[string]any) (result map[string]any, errText string) {
	a.w.t.Helper()
	res, err := a.session.CallTool(a.w.t.Context(), &mcp.CallToolParams{Name: name, Arguments: args})
	require.NoError(a.w.t, err, name)
	require.Len(a.w.t, res.Content, 1, name)
	tc, isText := res.Content[0].(*mcp.TextContent)
	require.True(a.w.t, isText, name)
	if res.IsError {
		return nil, tc.Text
	}
	var out map[string]any
	require.NoError(a.w.t, json.Unmarshal([]byte(tc.Text), &out), "%s: %s", name, tc.Text)
	return out, ""
}

func (a *Agent) mustCall(name string, args map[string]any) map[string]any {
	a.w.t.Helper()
	out, failure := a.call(name, args)
	require.Empty(a.w.t, failure, name)
	return out
}

// Draft is the message an agent is writing: a subject and the editor's document.
type Draft struct {
	Subject string
	Doc     map[string]any
}

// CopyStarter copies the starter Template of that name: the agent reads the Brand kit and the
// schema, finds the Template in the list, reads it with its document and writes a new Template
// from it, as there is no server-side copy.
func (a *Agent) CopyStarter(name string) Draft {
	a.w.t.Helper()
	a.mustCall("brand_get", map[string]any{})
	a.mustCall("email_schema", map[string]any{})

	list := a.mustCall("templates_list", map[string]any{})
	var id string
	items, isList := list["items"].([]any)
	require.True(a.w.t, isList, "templates_list returns its items")
	for _, item := range items {
		tpl, isTemplate := item.(map[string]any)
		require.True(a.w.t, isTemplate, "a listed Template is an object")
		if tpl["name"] == name {
			id = jsonID(tpl["id"])
		}
	}
	require.NotEmpty(a.w.t, id, "the starter Template %q is listed", name)

	tpl := a.mustCall("templates_get", map[string]any{"id": id})
	doc, isDoc := tpl["doc"].(map[string]any)
	require.True(a.w.t, isDoc, "the starter Template carries its document")
	copied := a.mustCall("templates_create", map[string]any{"name": name + " (copy)", "subject": tpl["subject"], "doc": doc})
	subject, isSubject := copied["subject"].(string)
	require.True(a.w.t, isSubject, "the copy carries its subject")
	copiedDoc, isCopiedDoc := copied["doc"].(map[string]any)
	require.True(a.w.t, isCopiedDoc, "the copy carries its document")
	return Draft{Subject: subject, Doc: copiedDoc}
}

// Edit replaces text in the draft's document: the first text node that contains old.
func (d *Draft) Edit(old, replacement string) bool { return replaceText(d.Doc, old, replacement) }

func replaceText(node map[string]any, old, replacement string) bool {
	if s, hasText := node["text"].(string); hasText && strings.Contains(s, old) {
		node["text"] = strings.Replace(s, old, replacement, 1)
		return true
	}
	children, _ := node["content"].([]any)
	for _, c := range children {
		if child, isNode := c.(map[string]any); isNode && replaceText(child, old, replacement) {
			return true
		}
	}
	return false
}

// Lint is the codes of what email_lint found in the draft.
type Lint struct {
	Errors   []string
	Warnings []string
}

// Lint runs email_lint over the draft.
func (a *Agent) Lint(d Draft) Lint {
	a.w.t.Helper()
	out := a.mustCall("email_lint", map[string]any{"subject": d.Subject, "doc": d.Doc})
	return Lint{Errors: codesOf(a.w.t, out["errors"]), Warnings: codesOf(a.w.t, out["warnings"])}
}

func codesOf(tb testing.TB, v any) []string {
	tb.Helper()
	findings, isList := v.([]any)
	require.True(tb, isList, "the findings are a list")
	codes := make([]string, 0, len(findings))
	for _, f := range findings {
		finding, isFinding := f.(map[string]any)
		require.True(tb, isFinding, "a finding is an object")
		code, isCode := finding["code"].(string)
		require.True(tb, isCode, "a finding carries its code")
		codes = append(codes, code)
	}
	return codes
}

// SaveBroadcast writes the draft as a Broadcast from the Workspace's sender and returns its id.
// It is a draft: sending is a person's activation.
func (a *Agent) SaveBroadcast(d Draft) externalapi.EntityId {
	a.w.t.Helper()
	out := a.mustCall("broadcasts_create", map[string]any{
		"name": "e2e agent draft " + uniq(), "subject": d.Subject, "doc": d.Doc,
		"fromName": a.w.FromName, "fromEmail": a.w.FromEmail,
	})
	require.Equal(a.w.t, "draft", out["status"])
	id, err := strconv.ParseInt(jsonID(out["id"]), 10, 64)
	require.NoError(a.w.t, err)
	return externalapi.EntityId(id)
}

// TrySchedule tries to schedule the Broadcast and returns why the tool refused, or "" when it ran.
func (a *Agent) TrySchedule(id externalapi.EntityId) string {
	a.w.t.Helper()
	_, failure := a.call("broadcasts_schedule", map[string]any{"id": jsonID(float64(id)), "scheduledAt": "2099-01-01T00:00:00Z"})
	return failure
}

// CanSend reports whether the agent is offered any send-class tool.
func (a *Agent) CanSend() bool {
	return slices.ContainsFunc(a.Tools(), func(name string) bool {
		return slices.Contains([]string{"emails_send", "broadcasts_schedule", "broadcasts_unschedule", "automations_publish", "rules_publish"}, name)
	})
}

// jsonID renders an id from a JSON result (a number or a string) as the string a path takes.
func jsonID(v any) string {
	switch id := v.(type) {
	case float64:
		return strconv.FormatInt(int64(id), 10)
	case string:
		return id
	}
	return ""
}
