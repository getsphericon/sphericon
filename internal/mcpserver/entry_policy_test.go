package mcpserver_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/internal/domain/identity/apitokens"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// An agent writes an Event Trigger's Entry policy and cooldown and reads them back on the version
// (ADR 0127).
func TestMCPWritesTheEntryPolicyAndCooldown(t *testing.T) {
	env := testhelper.Setup(t)
	s := env.MCPClient(t, env.ScopedBearer(t, apitokens.ScopeAutomationsRead, apitokens.ScopeAutomationsWrite))

	created := call(t, s, "automations_create", map[string]any{"name": "Post-purchase", "trigger": map[string]any{
		"kind": "event", "action": "order.completed", "entryPolicy": "again", "cooldown": 7200,
	}})

	require.False(t, created.IsError, text(t, created))
	trigger := decode(t, text(t, created))["draft"].(map[string]any)["trigger"].(map[string]any)
	assert.Equal(t, "again", trigger["entryPolicy"])
	assert.EqualValues(t, 7200, trigger["cooldown"])
}

// An agent reads each of a Contact's Enrollments with its ordinal (fixture Repeat purchase:
// Alice's two).
func TestMCPEnrollmentsShowTheirOrdinal(t *testing.T) {
	env := testhelper.Setup(t)
	s := env.MCPClient(t, env.ScopedBearer(t, apitokens.ScopeEnrollmentsRead))

	res := call(t, s, "enrollments_get", map[string]any{
		"automationId": testhelper.ID(fixtures.AutomationRepeatPurchaseID), "enrollmentId": testhelper.ID(fixtures.AutomationRunRepeatPurchaseSecondID),
	})

	require.False(t, res.IsError, text(t, res))
	assert.EqualValues(t, 2, decode(t, text(t, res))["ordinal"])
}

// An agent writes an Event Trigger's Enrollment key and reads it back on the version, and reads an
// Enrollment of a keyed Automation with its key value (ADR 0165; fixture Order series: Alice's
// order A).
func TestMCPWritesTheEnrollmentKeyAndReadsTheKeyValue(t *testing.T) {
	env := testhelper.Setup(t)
	s := env.MCPClient(t, env.ScopedBearer(t, apitokens.ScopeAutomationsRead, apitokens.ScopeAutomationsWrite, apitokens.ScopeEnrollmentsRead))

	created := call(t, s, "automations_create", map[string]any{"name": "Post-purchase", "trigger": map[string]any{
		"kind": "event", "action": "order.completed", "enrollmentKey": "order_id",
	}})
	enrollment := call(t, s, "enrollments_get", map[string]any{
		"automationId": testhelper.ID(fixtures.AutomationOrderSeriesID), "enrollmentId": testhelper.ID(fixtures.AutomationRunOrderSeriesAID),
	})

	require.False(t, created.IsError, text(t, created))
	trigger := decode(t, text(t, created))["draft"].(map[string]any)["trigger"].(map[string]any)
	assert.Equal(t, "order_id", trigger["enrollmentKey"])
	require.False(t, enrollment.IsError, text(t, enrollment))
	assert.Equal(t, map[string]any{"untrusted_data": "A"}, decode(t, text(t, enrollment))["keyValue"], "an Event's value, marked untrusted")
}
