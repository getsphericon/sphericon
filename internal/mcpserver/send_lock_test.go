package mcpserver_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent/automationrun"
	"github.com/getsphericon/sphericon/ent/contact"
	"github.com/getsphericon/sphericon/ent/rulefiring"
	"github.com/getsphericon/sphericon/internal/domain/identity/apitokens"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/jobs"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// writerScopes are what a Connection that tidies Contacts and records history is granted: its
// writes need no send.
var writerScopes = []apitokens.Scope{apitokens.ScopeContactsRead, apitokens.ScopeContactsWrite, apitokens.ScopeEventsWrite}

// withSend are writerScopes and the send lock's key.
var withSend = append([]apitokens.Scope{apitokens.ScopeMcpSend}, writerScopes...)

// halcyonWriter is an MCP session of a Connection of Halcyon's owner with exactly scopes.
func halcyonWriter(t *testing.T, env *testhelper.TestEnv, scopes []apitokens.Scope) func(tool string, args map[string]any) {
	t.Helper()
	s := env.MCPClient(t, env.ConnectionBearerIn(t, fixtures.HalcyonID, fixtures.HalcyonOwnerHalEmail, scopes...))
	return func(tool string, args map[string]any) {
		t.Helper()
		res := call(t, s, tool, args)
		require.False(t, res.IsError, text(t, res))
	}
}

// reactTo works what the Events published so far queue against the Automations and the Rules.
func reactTo(t *testing.T, env *testhelper.TestEnv) {
	t.Helper()
	env.PerformEnqueuedJobs(t, jobs.EvaluateTriggerArgs{}, jobs.EvaluateRulesArgs{})
}

// started counts what the write started for the Halcyon Contact with email: its Enrollments in the
// Automation and the Firings of the Rule on every Event a write publishes.
func started(t *testing.T, env *testhelper.TestEnv, email string, automationID int64) (enrollments, firings int) {
	t.Helper()
	ctx := t.Context()
	written, err := env.DB.Contact.Query().Where(contact.WorkspaceID(fixtures.HalcyonID), contact.Email(email)).Only(ctx)
	require.NoError(t, err)
	enrollments, err = env.DB.AutomationRun.Query().
		Where(automationrun.AutomationID(automationID), automationrun.ContactID(written.ID)).Count(ctx)
	require.NoError(t, err)
	firings, err = env.DB.RuleFiring.Query().
		Where(rulefiring.RuleID(fixtures.RuleHalcyonAgentWritesID), rulefiring.ContactID(written.ID)).Count(ctx)
	require.NoError(t, err)
	return enrollments, firings
}

// sendLockCase is one write through a Connection, and what it starts.
type sendLockCase struct {
	scopes          []apitokens.Scope
	wantEnrollments int
	wantFirings     int
}

// sendLockCases are a Connection without mcp:send, whose writes start nothing that sends (ADR
// 0132), and one with it, whose writes start what any other write does.
var sendLockCases = map[string]sendLockCase{
	"without mcp:send": {scopes: writerScopes, wantEnrollments: 0, wantFirings: 0},
	"with mcp:send":    {scopes: withSend, wantEnrollments: 1, wantFirings: 1},
}

// An Event recorded over MCP through a Connection without mcp:send enrolls no one and fires no
// Rule; with mcp:send it does both. Fixtures: Halcyon's Automation and Rule on agent_signal.
func TestAnEventRecordedThroughAConnectionStartsASendOnlyWithMCPSend(t *testing.T) {
	signal := map[string]any{"subjectId": "ida", "email": fixtures.ContactHalcyonIdaEmail, "action": "agent_signal"}
	for name, c := range sendLockCases {
		t.Run(name+", events_record", func(t *testing.T) {
			env := testhelper.Setup(t)
			write := halcyonWriter(t, env, c.scopes)

			write("events_record", map[string]any{"events": []any{signal}})
			reactTo(t, env)
			enrollments, firings := started(t, env, fixtures.ContactHalcyonIdaEmail, fixtures.AutomationHalcyonOnSignalID)

			assert.Equal(t, c.wantEnrollments, enrollments)
			assert.Equal(t, c.wantFirings, firings)
		})
		t.Run(name+", events_record_batch", func(t *testing.T) {
			env := testhelper.Setup(t)
			write := halcyonWriter(t, env, c.scopes)

			write("events_record_batch", map[string]any{"events": []any{signal}})
			reactTo(t, env)
			enrollments, firings := started(t, env, fixtures.ContactHalcyonIdaEmail, fixtures.AutomationHalcyonOnSignalID)

			assert.Equal(t, c.wantEnrollments, enrollments)
			assert.Equal(t, c.wantFirings, firings)
		})
	}
}

// A Tag applied through a Connection without mcp:send publishes a contact.tagged that enrolls no
// one and fires no Rule. Fixtures: Halcyon's Automation and Rule on contact.tagged.
func TestATagAppliedThroughAConnectionStartsASendOnlyWithMCPSend(t *testing.T) {
	for name, c := range sendLockCases {
		t.Run(name, func(t *testing.T) {
			env := testhelper.Setup(t)
			write := halcyonWriter(t, env, c.scopes)

			write("tags_apply", map[string]any{"contactId": testhelper.ID(fixtures.ContactHalcyonIdaID), "name": "agent-picked"})
			reactTo(t, env)
			enrollments, firings := started(t, env, fixtures.ContactHalcyonIdaEmail, fixtures.AutomationHalcyonOnTaggedID)

			assert.Equal(t, c.wantEnrollments, enrollments)
			assert.Equal(t, c.wantFirings, firings)
		})
	}
}

// A Custom field changed through a Connection without mcp:send publishes a contact.field_changed
// that enrolls no one and fires no Rule. Fixtures: Halcyon's mood field, and its Automation and
// Rule on contact.field_changed.
func TestACustomFieldChangedThroughAConnectionStartsASendOnlyWithMCPSend(t *testing.T) {
	for name, c := range sendLockCases {
		t.Run(name, func(t *testing.T) {
			env := testhelper.Setup(t)
			write := halcyonWriter(t, env, c.scopes)

			write("contacts_update", map[string]any{
				"id": testhelper.ID(fixtures.ContactHalcyonIdaID), "revision": 1, "customFields": map[string]any{"mood": "sunny"},
			})
			reactTo(t, env)
			enrollments, firings := started(t, env, fixtures.ContactHalcyonIdaEmail, fixtures.AutomationHalcyonOnFieldChangedID)

			assert.Equal(t, c.wantEnrollments, enrollments)
			assert.Equal(t, c.wantFirings, firings)
		})
	}
}

// A Contact created or upserted through a Connection without mcp:send publishes a contact.created
// that enrolls no one and fires no Rule. Fixtures: Halcyon's Automation and Rule on
// contact.created.
func TestAContactCreatedThroughAConnectionStartsASendOnlyWithMCPSend(t *testing.T) {
	const email = "new@halcyon.example"
	for name, c := range sendLockCases {
		t.Run(name+", contacts_create", func(t *testing.T) {
			env := testhelper.Setup(t)
			write := halcyonWriter(t, env, c.scopes)

			write("contacts_create", map[string]any{"email": email})
			reactTo(t, env)
			enrollments, firings := started(t, env, email, fixtures.AutomationHalcyonOnCreatedID)

			assert.Equal(t, c.wantEnrollments, enrollments)
			assert.Equal(t, c.wantFirings, firings)
		})
		t.Run(name+", contacts_upsert_batch", func(t *testing.T) {
			env := testhelper.Setup(t)
			write := halcyonWriter(t, env, c.scopes)

			write("contacts_upsert_batch", map[string]any{"contacts": []any{map[string]any{"email": email}}})
			reactTo(t, env)
			enrollments, firings := started(t, env, email, fixtures.AutomationHalcyonOnCreatedID)

			assert.Equal(t, c.wantEnrollments, enrollments)
			assert.Equal(t, c.wantFirings, firings)
		})
	}
}

// The mark only stops sends from starting: an Event recorded through a Connection without
// mcp:send still ends an active Enrollment as its goal or as an exit. Fixture: Ida's Enrollment
// in Halcyon's course, whose exit is halcyon_left and whose goal is halcyon_paid.
func TestAnEventRecordedThroughAConnectionWithoutMCPSendStillEndsAnEnrollment(t *testing.T) {
	for action, want := range map[string]automationrun.Status{
		"halcyon_paid": automationrun.StatusGoalReached,
		"halcyon_left": automationrun.StatusExited,
	} {
		t.Run(action, func(t *testing.T) {
			env := testhelper.Setup(t)
			write := halcyonWriter(t, env, writerScopes)

			write("events_record", map[string]any{"events": []any{
				map[string]any{"subjectId": "ida", "email": fixtures.ContactHalcyonIdaEmail, "action": action},
			}})
			reactTo(t, env)
			run, err := env.DB.AutomationRun.Get(t.Context(), fixtures.AutomationRunHalcyonCourseIdaID)
			require.NoError(t, err)

			assert.Equal(t, want, run.Status)
		})
	}
}

// The mark only stops sends from starting: an Event recorded through a Connection without
// mcp:send is still delivered to a Webhook endpoint subscribed to it. Fixture: Halcyon's endpoint,
// which receives every Event.
func TestAnEventRecordedThroughAConnectionWithoutMCPSendStillReachesAWebhookEndpoint(t *testing.T) {
	env := testhelper.Setup(t)
	write := halcyonWriter(t, env, writerScopes)

	write("events_record", map[string]any{"events": []any{
		map[string]any{"subjectId": "ida", "email": fixtures.ContactHalcyonIdaEmail, "action": "agent_signal"},
	}})
	env.PerformEnqueuedJobs(t, jobs.WebhookEventArgs{})
	deliveries := env.QueuedDeliveries(t)

	require.Len(t, deliveries, 1)
	assert.EqualValues(t, fixtures.WebhookHalcyonID, deliveries[0].EndpointID)
	assert.Equal(t, "agent_signal", deliveries[0].EventName)
}
