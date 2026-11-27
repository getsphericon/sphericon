package mcpserver_test

import (
	"testing"
	"time"

	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/automationrun"
	"github.com/getsphericon/sphericon/ent/contact"
	"github.com/getsphericon/sphericon/ent/rulefiring"
	"github.com/getsphericon/sphericon/internal/domain/identity/apitokens"
	"github.com/getsphericon/sphericon/internal/domain/orchestration/automations"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/jobs"
	"github.com/getsphericon/sphericon/internal/kernel/automationstep"
	"github.com/getsphericon/sphericon/internal/kernel/eventmatcher"
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

// A Confirmation recorded through a Connection without mcp:send carries the mark as the Contact
// written with it does (ADR 0132, ADR 0138): its marketing.confirmed enrolls no one and fires no
// Rule, from an upsert item's emailConsent and from confirmations_create alike. Fixtures:
// Halcyon's Automation and Rule on marketing.confirmed.
func TestAConfirmationRecordedThroughAConnectionStartsASendOnlyWithMCPSend(t *testing.T) {
	evidence := map[string]any{"wording": "Mail me the course", "source": "old platform", "brand": "Halcyon"}
	for name, c := range sendLockCases {
		t.Run(name+", contacts_upsert_batch", func(t *testing.T) {
			env := testhelper.Setup(t)
			write := halcyonWriter(t, env, c.scopes)

			write("contacts_upsert_batch", map[string]any{"contacts": []any{
				map[string]any{"email": fixtures.ContactHalcyonIdaEmail, "emailConsent": evidence},
			}})
			reactTo(t, env)
			enrollments, firings := started(t, env, fixtures.ContactHalcyonIdaEmail, fixtures.AutomationHalcyonOnConfirmedID)

			assert.Equal(t, c.wantEnrollments, enrollments)
			assert.Equal(t, c.wantFirings, firings)
		})
		t.Run(name+", confirmations_create", func(t *testing.T) {
			env := testhelper.Setup(t)
			write := halcyonWriter(t, env, c.scopes)

			write("confirmations_create", lo.Assign(evidence, map[string]any{
				"channel": "email", "destination": fixtures.ContactHalcyonIdaEmail,
			}))
			reactTo(t, env)
			enrollments, firings := started(t, env, fixtures.ContactHalcyonIdaEmail, fixtures.AutomationHalcyonOnConfirmedID)

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

// An Event recorded over MCP through a Connection without mcp:send never satisfies a Wait for
// event (ADR 0132): it leaves the Enrollment waiting, and the timeout's lookup skips it, so the
// Enrollment takes `timed_out`. Recorded with mcp:send, it takes `arrived` at once. Ida is enrolled
// on halcyon_trial into an Automation that waits an hour for halcyon_bought.
func TestAnEventRecordedThroughAConnectionWithoutMCPSendSatisfiesNoWaitForEvent(t *testing.T) {
	for name, c := range map[string]struct {
		scopes []apitokens.Scope
		// onArrival is the Step the Event leaves the Enrollment on, and took the path its wait took.
		onArrival, took func(wait automationstep.Step) automationstep.ID
	}{
		"without mcp:send": {
			scopes:    writerScopes,
			onArrival: func(wait automationstep.Step) automationstep.ID { return wait.ID },
			took: func(wait automationstep.Step) automationstep.ID {
				return wait.Payload.(automationstep.WaitForEvent).TimedOut.ID
			},
		},
		"with mcp:send": {
			scopes: withSend,
			onArrival: func(wait automationstep.Step) automationstep.ID {
				return wait.Payload.(automationstep.WaitForEvent).Arrived.Steps[0].ID
			},
			took: func(wait automationstep.Step) automationstep.ID {
				return wait.Payload.(automationstep.WaitForEvent).Arrived.ID
			},
		},
	} {
		t.Run(name, func(t *testing.T) {
			clock := testhelper.NewClock(time.Now().UTC().Add(-3 * time.Hour).Truncate(time.Second))
			env := testhelper.Setup(t, testhelper.WithClock(clock.Now))
			ctx := t.Context()
			halcyon := env.DB.Scoped(fixtures.HalcyonID)
			automationID, wait := waitingForAPurchase(t, env)
			record := func(write func(string, map[string]any), action string, at time.Time) {
				t.Helper()
				write("events_record", map[string]any{"events": []any{map[string]any{
					"subjectId": "ida", "email": fixtures.ContactHalcyonIdaEmail, "action": action, "occurredAt": at.Format(time.RFC3339),
				}}})
				reactTo(t, env)
			}

			record(halcyonWriter(t, env, withSend), "halcyon_trial", clock.Now())
			run, err := env.DB.AutomationRun.Query().
				Where(automationrun.AutomationID(automationID), automationrun.ContactID(fixtures.ContactHalcyonIdaID)).Only(ctx)
			require.NoError(t, err)
			record(halcyonWriter(t, env, c.scopes), "halcyon_bought", clock.Now().Add(time.Minute))
			onArrival, err := env.DB.AutomationRun.Get(ctx, run.ID)
			require.NoError(t, err)
			clock.Advance(2 * time.Hour)
			require.NoError(t, testhelper.Module[*automations.StepExecution](env).Step(ctx, halcyon, run.ID, wait.ID))
			records, err := testhelper.Module[*automations.Module](env).StepRecords(ctx, halcyon, automationID, run.ID)
			require.NoError(t, err)
			ofTheWait, ok := lo.Find(records, func(r *ent.AutomationStepRecord) bool { return r.StepID == wait.ID })
			require.True(t, ok)

			assert.Equal(t, c.onArrival(wait), lo.FromPtr(onArrival.CurrentStepID), "where the Event left it")
			assert.Equal(t, c.took(wait), lo.FromPtr(ofTheWait.PathID), "the path it took")
		})
	}
}

// waitingForAPurchase publishes in Halcyon an Automation triggered on halcyon_trial whose one Step
// waits an hour for halcyon_bought, tagging `buyer` on arrived and `reminded` on timed out: its id
// and its stored Wait for event.
func waitingForAPurchase(t *testing.T, env *testhelper.TestEnv) (int64, automationstep.Step) {
	t.Helper()
	ctx := t.Context()
	halcyon := env.DB.Scoped(fixtures.HalcyonID)
	m := testhelper.Module[*automations.Module](env)
	a, err := m.Create(ctx, halcyon, automations.CreateInput{
		Name: "Waits for a purchase", Trigger: automations.EventTrigger{Matcher: eventmatcher.Matcher{Action: "halcyon_trial"}},
		Steps: new(automationstep.Root(automationstep.New(automationstep.WaitForEvent{
			Name: "purchase", TimeoutSeconds: 3600,
			Awaits:   automationstep.AwaitedEvent{Matchers: []eventmatcher.Matcher{{Action: "halcyon_bought"}}},
			Arrived:  automationstep.Root(automationstep.New(automationstep.ApplyTag{Tag: "buyer"})),
			TimedOut: automationstep.Root(automationstep.New(automationstep.ApplyTag{Tag: "reminded"})),
		}))),
	})
	require.NoError(t, err)
	_, err = m.Publish(ctx, halcyon, a.ID, testhelper.AutomationRevisionIn(ctx, t, halcyon, a.ID))
	require.NoError(t, err)
	live, err := m.Get(ctx, halcyon, a.ID)
	require.NoError(t, err)
	version, _ := automations.LiveAndDraft(live)
	return a.ID, version.Steps.Steps[0]
}
