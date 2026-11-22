package mcpserver_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/internal/domain/identity/apitokens"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// An agent reads a Contact's skipped entries into an Automation with automations:read, the Event's
// source id, which the caller who recorded it wrote, wrapped as untrusted (ADR 0127, ADR 0016).
func TestMCPSkippedEntriesOfAContactWrapTheEventDataAsUntrusted(t *testing.T) {
	env := testhelper.Setup(t)
	s := env.MCPClient(t, env.ScopedBearer(t, apitokens.ScopeAutomationsRead))

	res := call(t, s, "automations_skipped_entries_list", map[string]any{
		"automationId": testhelper.ID(fixtures.AutomationWelcomeSeriesID), "contactId": testhelper.ID(fixtures.ContactEmmaID),
	})

	require.False(t, res.IsError, text(t, res))
	items := decode(t, text(t, res))["items"].([]any)
	require.Len(t, items, 1)
	entry := items[0].(map[string]any)
	assert.Equal(t, "active_enrollment", entry["reason"])
	assert.Equal(t, map[string]any{"untrusted_data": "fixture:skipped-welcome-emma"}, entry["eventSourceId"])
}

func TestMCPSkippedEntriesNeedAutomationsRead(t *testing.T) {
	env := testhelper.Setup(t)
	s := env.MCPClient(t, env.ScopedBearer(t, apitokens.ScopeEnrollmentsRead))

	res := call(t, s, "automations_skipped_entries_list", map[string]any{"automationId": testhelper.ID(fixtures.AutomationWelcomeSeriesID)})

	assert.True(t, res.IsError)
}
