package mcpserver_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/getsphericon/sphericon/ent/broadcast"
	"github.com/getsphericon/sphericon/internal/credentials"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/testhelper"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func call(t *testing.T, s *mcp.ClientSession, name string, args map[string]any) *mcp.CallToolResult {
	t.Helper()
	res, err := s.CallTool(context.Background(), &mcp.CallToolParams{Name: name, Arguments: args})
	require.NoError(t, err)
	return res
}

func text(t *testing.T, res *mcp.CallToolResult) string {
	t.Helper()
	require.Len(t, res.Content, 1)
	tc, ok := res.Content[0].(*mcp.TextContent)
	require.Truef(t, ok, "got %T", res.Content[0])
	return tc.Text
}

func TestMCPRequiresBearerToken(t *testing.T) {
	env := testhelper.Setup(t)
	body := `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":{"name":"x","version":"0"}}}`

	for name, auth := range map[string][]string{
		"missing":   nil,
		"malformed": {"Bearer nonsense"},
		"unknown":   {"Bearer omtk_deadbeef_invalidsecret"},
	} {
		t.Run(name, func(t *testing.T) {
			req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, testhelper.MCPEndpoint, strings.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("Accept", "application/json, text/event-stream")
			req.Header["Authorization"] = auth
			rec := httptest.NewRecorder()
			env.Server.ServeHTTP(rec, req)
			assert.Equal(t, http.StatusUnauthorized, rec.Code)
		})
	}
}

// Independent literal: the /api operations that are not x-mcp hidden, by sphericon name.
// Token management is hidden. The send-class tools (ADR 0016, "Send is a second
// lock") are listed only for a token that also carries mcp:send.
var (
	authoringTools = []string{
		"segments_list", "segments_create", "segments_get", "segments_update", "segments_archive", "segments_unarchive", "segments_preview",
		"contacts_upsert_batch", "events_record_batch",
		"contacts_list", "contacts_create", "contacts_get", "contacts_update", "contacts_delete", "contacts_erase_by", "contacts_export",
		"broadcasts_list", "broadcasts_create", "broadcasts_get", "broadcasts_update", "broadcasts_delete",
		"broadcasts_set_segment", "broadcasts_test_send", "broadcasts_report", "broadcasts_archive", "broadcasts_unarchive",
		"events_record", "events_actions_list", "whoami", "workspace_get",
		"custom_fields_list", "sending_domains_list", "sending_domains_create", "sending_domains_get", "sending_domains_update", "sending_domains_archive", "sending_domains_unarchive", "sending_domains_verify", "sending_domains_rates",
		"integrations_list", "integrations_create", "integrations_get", "integrations_update", "integrations_archive", "integrations_unarchive", "presets_list",
		"captured_messages_list", "captured_messages_get", "captured_messages_delete",
		"suppressions_create", "unsubscribes_create",
		"email_lint", "email_schema", "brand_get", "templates_list", "templates_create", "templates_get", "templates_update", "templates_delete",
		"webhooks_list", "webhooks_create", "webhooks_get", "webhooks_update", "webhooks_delete",
		"webhooks_regenerate_secret", "webhooks_test",
		"tags_list", "tags_list_for_contact", "tags_apply", "tags_remove", "tags_delete",
		"automations_list", "automations_create", "automations_get", "automations_update", "automations_delete",
		"rules_list", "rules_create", "rules_get", "rules_update", "rules_dry_run", "rules_deactivate", "rules_revert", "rules_archive", "rules_unarchive", "rules_delete", "rule_firings_list",
		"automations_deactivate", "automations_revert", "automations_archive", "automations_unarchive", "automations_dry_run_http_step", "automations_report", "enrollments_list", "enrollments_get",
	}
	sendTools = []string{"emails_send", "broadcasts_schedule", "broadcasts_unschedule", "automations_publish", "rules_publish"}
)

func listedTools(t *testing.T, s *mcp.ClientSession) map[string]*mcp.Tool {
	t.Helper()
	list, err := s.ListTools(context.Background(), nil)
	require.NoError(t, err)
	got := map[string]*mcp.Tool{}
	for _, tool := range list.Tools {
		got[tool.Name] = tool
	}
	return got
}

func toolNames(got map[string]*mcp.Tool) []string {
	names := make([]string, 0, len(got))
	for n := range got {
		names = append(names, n)
	}
	return names
}

func TestMCPSendToolsAreListedOnlyWithMCPSend(t *testing.T) {
	env := testhelper.Setup(t)

	// The send scopes alone do not list them: mcp:send is the second lock.
	apiOnly := listedTools(t, env.MCPClient(t, env.ScopedBearer(t, "emails:send", "broadcasts:send")))
	assert.ElementsMatch(t, authoringTools, toolNames(apiOnly))

	// mcp:send alone lists them too (the /api scope still gates the call).
	withSend := listedTools(t, env.MCPClient(t, env.ScopedBearer(t, "mcp:send")))
	assert.ElementsMatch(t, append(slices.Clone(authoringTools), sendTools...), toolNames(withSend))
}

func TestMCPSendToolCallIsRefusedWithoutMCPSend(t *testing.T) {
	env := testhelper.Setup(t)
	args := map[string]any{"id": strconv.Itoa(fixtures.BroadcastDraftID), "scheduledAt": "2099-01-01T00:00:00Z"}

	// Direct call, though the token has the /api scope: refused by the MCP lock.
	s := env.MCPClient(t, env.ScopedBearer(t, "broadcasts:send"))
	res := call(t, s, "broadcasts_schedule", args)
	require.True(t, res.IsError)
	assert.Contains(t, text(t, res), "mcp:send")
	b, err := env.DB.Broadcast.Get(context.Background(), fixtures.BroadcastDraftID)
	require.NoError(t, err)
	assert.Equal(t, broadcast.StatusDraft, b.Status, "the refused call changed nothing")

	// mcp:send without the /api scope: passes the MCP lock, refused by /api.
	s = env.MCPClient(t, env.ScopedBearer(t, "mcp:send"))
	res = call(t, s, "broadcasts_schedule", args)
	require.True(t, res.IsError)
	assert.Contains(t, text(t, res), "HTTP 403 Forbidden")
	assert.Contains(t, text(t, res), "broadcasts:send")

	// Both: the broadcast is scheduled.
	s = env.MCPClient(t, env.ScopedBearer(t, "mcp:send", "broadcasts:send"))
	res = call(t, s, "broadcasts_schedule", args)
	require.False(t, res.IsError, text(t, res))
	b, err = env.DB.Broadcast.Get(context.Background(), fixtures.BroadcastDraftID)
	require.NoError(t, err)
	assert.Equal(t, broadcast.StatusScheduled, b.Status)

	res = call(t, s, "broadcasts_unschedule", map[string]any{"id": strconv.Itoa(fixtures.BroadcastDraftID)})
	require.False(t, res.IsError, text(t, res))
	b, err = env.DB.Broadcast.Get(context.Background(), fixtures.BroadcastDraftID)
	require.NoError(t, err)
	assert.Equal(t, broadcast.StatusDraft, b.Status)
}

// Publishing an Automation or a Rule is send-class (ADR 0065): the publish scope alone is refused
// by the MCP lock, and with mcp:send it runs. Deactivating is authoring: the write scope is enough.
func TestMCPPublishNeedsMCPSendAndDeactivateDoesNot(t *testing.T) {
	env := testhelper.Setup(t)
	automationID := map[string]any{"id": strconv.Itoa(fixtures.AutomationWinBackID)}
	ruleID := map[string]any{"id": strconv.Itoa(fixtures.RuleInactiveID)}

	publishOnly := env.MCPClient(t, env.ScopedBearer(t, "automations:read", "automations:publish"))
	refusedAutomation := call(t, publishOnly, "automations_publish", automationID)
	refusedRule := call(t, publishOnly, "rules_publish", ruleID)
	unchanged := text(t, call(t, publishOnly, "automations_get", automationID))
	assert.True(t, refusedAutomation.IsError)
	assert.Contains(t, text(t, refusedAutomation), "mcp:send")
	assert.True(t, refusedRule.IsError)
	assert.Contains(t, text(t, refusedRule), "mcp:send")
	assert.Contains(t, unchanged, `"status":"inactive"`, "the refused call changed nothing")

	publisher := env.MCPClient(t, env.ScopedBearer(t, "automations:publish", "mcp:send"))
	publishedAutomation := call(t, publisher, "automations_publish", automationID)
	publishedRule := call(t, publisher, "rules_publish", ruleID)
	require.False(t, publishedAutomation.IsError, text(t, publishedAutomation))
	require.False(t, publishedRule.IsError, text(t, publishedRule))
	assert.Contains(t, text(t, publishedAutomation), `"status":"active"`)
	assert.Contains(t, text(t, publishedRule), `"status":"active"`)

	author := env.MCPClient(t, env.ScopedBearer(t, "automations:write"))
	deactivatedAutomation := call(t, author, "automations_deactivate", automationID)
	deactivatedRule := call(t, author, "rules_deactivate", ruleID)
	require.False(t, deactivatedAutomation.IsError, text(t, deactivatedAutomation))
	require.False(t, deactivatedRule.IsError, text(t, deactivatedRule))
	assert.Contains(t, text(t, deactivatedAutomation), `"status":"inactive"`)
	assert.Contains(t, text(t, deactivatedRule), `"status":"inactive"`)
}

// Regenerating an Integration's hook key is hidden from agents: the new URL is a secret an
// agent should not be handed, and the old one stops working at once. The Presets list is a tool.
func TestMCPRegenerateKeyIsHiddenAndPresetsListIsATool(t *testing.T) {
	env := testhelper.Setup(t)
	got := listedTools(t, env.MCPClient(t, env.ScopedBearer(t, "integrations:read")))

	for _, name := range toolNames(got) {
		assert.NotContains(t, name, "integrations_regenerate")
	}
	assert.Contains(t, toolNames(got), "presets_list")
}

// Captured messages can be listed, read and deleted one by one by an agent, but wiping all of
// them in one call needs a person (ADR 0058).
func TestMCPCapturedMessagesHideDeleteAll(t *testing.T) {
	env := testhelper.Setup(t)
	got := toolNames(listedTools(t, env.MCPClient(t, env.ScopedBearer(t, "captured_messages:read", "captured_messages:write"))))

	assert.Contains(t, got, "captured_messages_list")
	assert.Contains(t, got, "captured_messages_get")
	assert.Contains(t, got, "captured_messages_delete")
	assert.NotContains(t, got, "captured_messages_delete_all")
}

func TestMCPToolsAreTheContractMinusHiddenOperations(t *testing.T) {
	env := testhelper.Setup(t)
	s := env.MCPClient(t, env.ScopedBearer(t, "contacts:read"))
	got := listedTools(t, s)
	assert.ElementsMatch(t, authoringTools, toolNames(got))

	// Hints derive from the HTTP method.
	ro := got["contacts_list"].Annotations
	require.NotNil(t, ro)
	assert.True(t, ro.ReadOnlyHint)

	del := got["contacts_delete"].Annotations
	require.NotNil(t, del)
	assert.False(t, del.ReadOnlyHint)
	require.NotNil(t, del.DestructiveHint)
	assert.True(t, *del.DestructiveHint)
	assert.True(t, del.IdempotentHint)

	post := got["contacts_create"].Annotations
	require.NotNil(t, post)
	require.NotNil(t, post.DestructiveHint)
	assert.False(t, *post.DestructiveHint)
	assert.False(t, post.IdempotentHint)

	put := got["contacts_update"].Annotations
	require.NotNil(t, put)
	assert.True(t, put.IdempotentHint)

	assert.NotEmpty(t, got["contacts_list"].Description)
}

// The Audit log is not an agent capability (ADR 0016, ADR 0022): even a token that
// carries audit:read gets no tool for it.
func TestMCPDoesNotExposeTheAuditLog(t *testing.T) {
	env := testhelper.Setup(t)
	got := listedTools(t, env.MCPClient(t, env.ScopedBearer(t, "audit:read")))
	for name := range got {
		assert.NotContains(t, name, "audit")
	}
	assert.ElementsMatch(t, authoringTools, toolNames(got))
}

// An Export stream is a bulk feed, not an agent capability (ADR 0016, ADR 0045): even a token
// that carries export:read gets no tool for it.
func TestMCPDoesNotExposeTheExportStreams(t *testing.T) {
	env := testhelper.Setup(t)
	got := listedTools(t, env.MCPClient(t, env.ScopedBearer(t, "export:read")))
	assert.NotContains(t, toolNames(got), "export_events")
	assert.NotContains(t, toolNames(got), "export_erasures")
	assert.ElementsMatch(t, authoringTools, toolNames(got))
}

func TestMCPToolCallReturnsTheAPIResult(t *testing.T) {
	env := testhelper.Setup(t)
	s := env.MCPClient(t, env.ScopedBearer(t, "contacts:read", "contacts:write", "contacts:erase"))

	res := call(t, s, "contacts_get", map[string]any{"id": strconv.Itoa(fixtures.ContactAliceID)})
	require.False(t, res.IsError, text(t, res))
	var contact map[string]any
	require.NoError(t, json.Unmarshal([]byte(text(t, res)), &contact))
	assert.Equal(t, "alice@example.com", contact["email"], "fixture contact 1")

	// Query parameters.
	res = call(t, s, "contacts_list", map[string]any{"pageSize": 2})
	require.False(t, res.IsError, text(t, res))
	var page struct {
		Items    []map[string]any `json:"items"`
		PageSize int              `json:"pageSize"`
	}
	require.NoError(t, json.Unmarshal([]byte(text(t, res)), &page))
	assert.Equal(t, 2, page.PageSize)
	assert.Len(t, page.Items, 2)

	// Body fields are top-level arguments.
	res = call(t, s, "contacts_create", map[string]any{"email": "mcp-new@example.com", "firstName": "Mcp"})
	require.False(t, res.IsError, text(t, res))
	var created map[string]any
	require.NoError(t, json.Unmarshal([]byte(text(t, res)), &created))
	assert.Equal(t, "mcp-new@example.com", created["email"])

	// Path parameter and body together; 204 carries no body.
	res = call(t, s, "contacts_update", map[string]any{"id": created["id"], "firstName": "Renamed"})
	require.False(t, res.IsError, text(t, res))
	res = call(t, s, "contacts_delete", map[string]any{"id": created["id"]})
	require.False(t, res.IsError, text(t, res))
}

// A body an agent saves over MCP is sanitized like one saved through the API (ADR 0040).
func TestMCPSanitizesBodiesOnSave(t *testing.T) {
	env := testhelper.Setup(t)
	s := env.MCPClient(t, env.ScopedBearer(t, "templates:write"))

	res := call(t, s, "templates_create", map[string]any{
		"name": "Hostile",
		"body": `<p onclick="steal()">Hi {{ first_name }}</p><script>alert(1)</script>`,
	})
	require.False(t, res.IsError, text(t, res))
	var created map[string]any
	require.NoError(t, json.Unmarshal([]byte(text(t, res)), &created))
	assert.Equal(t, `<p>Hi {{ first_name }}</p>`, created["body"])
}

// An agent reads the Provider limit and 24-hour usage of the Integrations over MCP,
// with the same scope the /api read needs.
func TestMCPIntegrationsListReadsProviderLimitAndUsage(t *testing.T) {
	env := testhelper.Setup(t)
	env.DB.Integration.UpdateOneID(fixtures.IntegrationAcmeDefaultID).SetMaxPerSecond(14).ExecX(context.Background())

	denied := call(t, env.MCPClient(t, env.ScopedBearer(t, "contacts:read")), "integrations_list", nil)
	assert.True(t, denied.IsError, "integrations:read is required")

	res := call(t, env.MCPClient(t, env.ScopedBearer(t, "integrations:read")), "integrations_list", nil)
	require.False(t, res.IsError, text(t, res))
	var page struct {
		Items []struct {
			ProviderLimit struct {
				PerSecond struct {
					Limit  int    `json:"limit"`
					Source string `json:"source"`
				} `json:"perSecond"`
				SentLast24h int `json:"sentLast24h"`
			} `json:"providerLimit"`
		} `json:"items"`
	}
	require.NoError(t, json.Unmarshal([]byte(text(t, res)), &page))
	require.NotEmpty(t, page.Items)
	assert.Equal(t, 14, page.Items[0].ProviderLimit.PerSecond.Limit)
	assert.Equal(t, "manual", page.Items[0].ProviderLimit.PerSecond.Source)
	assert.Equal(t, 2, page.Items[0].ProviderLimit.SentLast24h)
}

// Erasure is irreversible: a token that can write Contacts cannot erase them through
// MCP either, since every tool call is the /api call under the caller's own scopes.
func TestMCPContactDeleteNeedsTheEraseScope(t *testing.T) {
	env := testhelper.Setup(t)
	s := env.MCPClient(t, env.ScopedBearer(t, "contacts:read", "contacts:write"))

	id := strconv.Itoa(fixtures.ContactAliceID)
	res := call(t, s, "contacts_delete", map[string]any{"id": id})
	assert.True(t, res.IsError, "contacts:write is not contacts:erase")
	res = call(t, s, "contacts_erase_by", map[string]any{"email": fixtures.ContactAliceEmail})
	assert.True(t, res.IsError)

	res = call(t, s, "contacts_get", map[string]any{"id": id})
	require.False(t, res.IsError, text(t, res))
}

// Tags are tools: a name with a slash travels as a path parameter, and the contact id
// as another.
func TestMCPTagToolsApplyAndRemove(t *testing.T) {
	env := testhelper.Setup(t)
	s := env.MCPClient(t, env.ScopedBearer(t, "contacts:read", "contacts:write"))

	res := call(t, s, "tags_apply", map[string]any{"contactId": "2", "name": "plan / pro"})
	require.False(t, res.IsError, text(t, res))
	assert.Contains(t, text(t, res), "plan / pro")

	res = call(t, s, "tags_list_for_contact", map[string]any{"contactId": "2"})
	require.False(t, res.IsError, text(t, res))
	assert.Contains(t, text(t, res), "plan / pro")

	res = call(t, s, "tags_remove", map[string]any{"contactId": "2", "name": "plan / pro"})
	require.False(t, res.IsError, text(t, res))

	res = call(t, s, "tags_list_for_contact", map[string]any{"contactId": "2"})
	require.False(t, res.IsError, text(t, res))
	assert.NotContains(t, text(t, res), "plan / pro")
}

func TestMCPToolCallReturnsTheAPIError(t *testing.T) {
	env := testhelper.Setup(t)
	ro := env.MCPClient(t, env.ScopedBearer(t, "contacts:read"))

	// Scope: the /api 403 for a token without the scope comes back as a tool error.
	res := call(t, ro, "contacts_create", map[string]any{"email": "nope@example.com"})
	require.True(t, res.IsError)
	assert.Contains(t, text(t, res), "HTTP 403 Forbidden")
	assert.Contains(t, text(t, res), "the API token lacks the scope contacts:write")

	// Not found.
	res = call(t, ro, "contacts_get", map[string]any{"id": "999999"})
	require.True(t, res.IsError)
	assert.Contains(t, text(t, res), "404")
}

func TestMCPCallsAreIsolatedToTheTokensWorkspace(t *testing.T) {
	env := testhelper.Setup(t)
	s := env.MCPClient(t, env.ScopedBearerFor(t, fixtures.GlobexID, "contacts:read"))

	res := call(t, s, "contacts_get", map[string]any{"id": strconv.Itoa(fixtures.ContactAliceID)})
	require.True(t, res.IsError, "the contact belongs to workspace acme")

	res = call(t, s, "contacts_list", nil)
	require.False(t, res.IsError, text(t, res))
	assert.NotContains(t, text(t, res), "alice@example.com")
}

// Consent only narrows through the agent (ADR 0016): MCP can add a Suppression or
// record an Unsubscribe, and exposes nothing that resubscribes or lifts one.
func TestMCPConsentOnlyNarrows(t *testing.T) {
	env := testhelper.Setup(t)
	s := env.MCPClient(t, env.ScopedBearer(t, "contacts:write", "suppressions:write"))

	list, err := s.ListTools(context.Background(), nil)
	require.NoError(t, err)
	for _, tool := range list.Tools {
		assert.NotContains(t, strings.ToLower(tool.Name), "resubscribe")
	}
	consentTools := lo.Filter(list.Tools, func(tool *mcp.Tool, _ int) bool {
		name := strings.ToLower(tool.Name)
		return strings.Contains(name, "suppression") || strings.Contains(name, "unsubscribe")
	})
	for _, tool := range consentTools {
		assert.True(t, strings.HasSuffix(strings.ToLower(tool.Name), "_create"), "%s: consent tools only create", tool.Name)
		require.NotNil(t, tool.Annotations)
		require.NotNil(t, tool.Annotations.DestructiveHint)
		assert.False(t, *tool.Annotations.DestructiveHint)
	}

	res := call(t, s, "unsubscribes_create", map[string]any{"destination": "alice@example.com"})
	require.False(t, res.IsError, text(t, res))
	assert.Contains(t, text(t, res), `"sendingSource":"broadcasts"`)

	res = call(t, s, "suppressions_create", map[string]any{"destination": "alice@example.com"})
	require.False(t, res.IsError, text(t, res))
	assert.Contains(t, text(t, res), `"reason":"manual"`)
}

// An agent manages an Integration over MCP with the /api scopes; the credential is
// write-only on this surface too.
func TestMCPIntegrationsAreManagedWithoutLeakingSecrets(t *testing.T) {
	env := testhelper.Setup(t)
	read := env.MCPClient(t, env.ScopedBearer(t, "integrations:read"))
	s := env.MCPClient(t, env.ScopedBearer(t, "integrations:read", "integrations:write"))

	denied := call(t, read, "integrations_create", map[string]any{
		"name": "x", "config": map[string]any{"kind": "smtp", "host": "smtp.example.com", "port": 587, "from": "a@acme.test"},
	})
	assert.True(t, denied.IsError, "integrations:write is required")

	res := call(t, s, "integrations_create", map[string]any{
		"name": "Agent SMTP",
		"config": map[string]any{
			"kind": "smtp", "host": "smtp.example.com", "port": 587, "from": "a@acme.test", "password": "mcp-secret-pw",
		},
	})
	require.False(t, res.IsError, text(t, res))
	assert.NotContains(t, text(t, res), "mcp-secret-pw")
	var created struct {
		ID string `json:"id"`
	}
	require.NoError(t, json.Unmarshal([]byte(text(t, res)), &created))

	res = call(t, s, "integrations_get", map[string]any{"id": created.ID})
	require.False(t, res.IsError, text(t, res))
	assert.NotContains(t, text(t, res), "mcp-secret-pw")

	res = call(t, s, "integrations_archive", map[string]any{"id": created.ID})
	require.False(t, res.IsError, text(t, res))
}

// An agent connecting an SES-compatible service over MCP is told it needs the Provider limit
// (ADR 0067), as /api tells any caller.
func TestMCPIntegrationsSesCompatibleRequiresTheProviderLimit(t *testing.T) {
	env := testhelper.Setup(t)
	s := env.MCPClient(t, env.ScopedBearer(t, "integrations:read", "integrations:write"))
	res := call(t, s, "integrations_create", map[string]any{
		"name":   "Postbox",
		"params": map[string]any{"topic_arn": "arn:aws:sns:eu-west-1:123456789012:acme-ses-events"},
		"config": map[string]any{
			"kind": "ses", "region": "ru-central1", "accessKeyId": "AKIA1234", "secretAccessKey": "secret",
			"from": "a@acme.test", "endpoint": "https://postbox.cloud.yandex.net",
		},
	})
	require.True(t, res.IsError)
	assert.Contains(t, text(t, res), "HTTP 422")
	assert.Contains(t, text(t, res), "maxPerSecond")
	assert.Contains(t, text(t, res), "maxPerDay")
}

// An agent sets up a Sending domain over MCP: create, verify, read back. The DKIM
// private key never appears, and writes need sending_domains:write.
func TestMCPSendingDomainsAreManagedWithoutLeakingTheKey(t *testing.T) {
	env := testhelper.Setup(t)
	read := env.MCPClient(t, env.ScopedBearer(t, "sending_domains:read"))
	s := env.MCPClient(t, env.ScopedBearer(t, "sending_domains:read", "sending_domains:write"))

	denied := call(t, read, "sending_domains_create", map[string]any{"domain": "agent.example.com"})
	assert.True(t, denied.IsError, "sending_domains:write is required")

	res := call(t, s, "sending_domains_create", map[string]any{"domain": "agent.example.com"})
	require.False(t, res.IsError, text(t, res))
	assert.NotContains(t, text(t, res), "PRIVATE KEY")
	var created struct {
		ID string `json:"id"`
	}
	require.NoError(t, json.Unmarshal([]byte(text(t, res)), &created))

	res = call(t, s, "sending_domains_verify", map[string]any{"id": created.ID})
	require.False(t, res.IsError, text(t, res))

	res = call(t, s, "sending_domains_get", map[string]any{"id": created.ID})
	require.False(t, res.IsError, text(t, res))
	assert.NotContains(t, text(t, res), "PRIVATE KEY")

	res = call(t, s, "sending_domains_archive", map[string]any{"id": created.ID})
	require.False(t, res.IsError, text(t, res))
}

// The Transactional send tool takes the same `track` field as the API (ADR 0053): the mail then
// carries the open pixel and its links go through the click tracker.
func TestMCPEmailsSendAcceptsTrack(t *testing.T) {
	env := testhelper.Setup(t)
	s := env.MCPClient(t, env.ScopedBearer(t, "mcp:send", "emails:send"))

	res := call(t, s, "emails_send", map[string]any{
		"templateId":  strconv.Itoa(fixtures.TemplateWithLinkID),
		"destination": fixtures.ContactAliceEmail,
		"track":       true,
	})
	require.False(t, res.IsError, text(t, res))

	msgs := env.CustomerMail.Messages()
	require.Len(t, msgs, 1)
	assert.Contains(t, msgs[0].HTML, "/e/o/")
	assert.Contains(t, msgs[0].HTML, "/e/c/")
}

// The dev token CONTRIBUTING.md gives an MCP client: it reads and writes Brightpath, and
// lists no send tool.
func TestMCPDevTokenReadsAndWritesBrightpath(t *testing.T) {
	env := testhelper.Setup(t)
	s := env.MCPClient(t, credentials.TokenValue(fixtures.BrightpathDevMcpTokenPrefix, fixtures.BrightpathDevMcpTokenSecret))

	names := toolNames(listedTools(t, s))
	assert.Contains(t, names, "contacts_create")
	assert.Empty(t, lo.Intersect(names, sendTools))

	read := call(t, s, "contacts_list", map[string]any{})
	require.False(t, read.IsError, text(t, read))
	assert.Contains(t, text(t, read), fixtures.BrightpathContactSofiaEmail)

	created := "dev.mcp@brightpath.example"
	write := call(t, s, "contacts_create", map[string]any{"email": created})
	require.False(t, write.IsError, text(t, write))
	assert.Contains(t, text(t, write), created)
}
