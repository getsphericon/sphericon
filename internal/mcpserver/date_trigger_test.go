package mcpserver_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/internal/domain/identity/apitokens"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// An agent writes a date Trigger (ADR 0126) and reads it back on the version, and one with an Entry
// policy is refused: a date Trigger takes none.
func TestMCPWritesADateTrigger(t *testing.T) {
	env := testhelper.Setup(t)
	s := env.MCPClient(t, env.ScopedBearer(t, apitokens.ScopeAutomationsRead, apitokens.ScopeAutomationsWrite))
	trigger := map[string]any{
		"kind": "date", "field": map[string]any{"kind": "customField", "key": "trial_ends_at"},
		"recurrence": "once", "offsetDays": -3, "localTime": "10:00",
	}

	created := call(t, s, "automations_create", map[string]any{"name": "Trial ending", "trigger": trigger})
	trigger["entryPolicy"] = "again"
	refused := call(t, s, "automations_create", map[string]any{"name": "Trial ending again", "trigger": trigger})

	require.False(t, created.IsError, text(t, created))
	read := decode(t, text(t, created))["draft"].(map[string]any)["trigger"].(map[string]any)
	assert.Equal(t, map[string]any{
		"kind": "date", "field": map[string]any{"kind": "customField", "key": "trial_ends_at"},
		"recurrence": "once", "offsetDays": float64(-3), "localTime": "10:00",
	}, read)
	assert.True(t, refused.IsError, text(t, refused))
}
