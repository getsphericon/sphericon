package mcpserver_test

import (
	"testing"
	"time"

	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent/automationrun"
	"github.com/getsphericon/sphericon/ent/automationskippedentry"
	"github.com/getsphericon/sphericon/internal/domain/identity/apitokens"
	"github.com/getsphericon/sphericon/internal/domain/orchestration/automations"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/kernel/datetrigger"
	"github.com/getsphericon/sphericon/internal/kernel/localcalendar"
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

// A birthday written through a Connection without mcp:send is send-locked (ADR 0132 as amended by
// ADR 0126): its occurrence enrolls no one and is skipped as `send_locked`; written through one with
// mcp:send, it enrolls the Contact.
func TestABirthdayWrittenThroughAConnectionStartsASendOnlyWithMCPSend(t *testing.T) {
	for name, c := range map[string]struct {
		scopes      []apitokens.Scope
		enrollments int
		skipped     []automationskippedentry.Reason
	}{
		"without mcp:send": {scopes: writerScopes, skipped: []automationskippedentry.Reason{automationskippedentry.ReasonSendLocked}},
		"with mcp:send":    {scopes: withSend, enrollments: 1},
	} {
		t.Run(name, func(t *testing.T) {
			clock := testhelper.NewClock(time.Date(2027, time.March, 13, 12, 0, 0, 0, time.UTC))
			env := testhelper.Setup(t, testhelper.WithClock(clock.Now))
			ctx := t.Context()
			acme := env.DB.Scoped(fixtures.AcmeID)
			m := testhelper.Module[*automations.Module](env)
			created, err := m.Create(ctx, acme, automations.CreateInput{Name: "Birthday", Trigger: automations.DateTrigger{Date: datetrigger.Date{
				Field: datetrigger.Field{Kind: datetrigger.Birthday}, Recurrence: localcalendar.Yearly, LocalTime: "10:00",
			}}})
			require.NoError(t, err)
			_, err = m.Publish(ctx, acme, created.ID, testhelper.AutomationRevisionIn(ctx, t, acme, created.ID))
			require.NoError(t, err)
			s := env.MCPClient(t, env.ConnectionBearer(t, fixtures.OwnerJohnEmail, c.scopes...))

			written := call(t, s, "contacts_update", map[string]any{
				"id": testhelper.ID(fixtures.ContactAliceID), "revision": 1, "birthday": "1990-03-14",
			})
			require.False(t, written.IsError, text(t, written))
			clock.Set(time.Date(2027, time.March, 14, 10, 5, 0, 0, time.UTC))
			require.NoError(t, testhelper.Module[*automations.StepExecution](env).DecideOccurrences(ctx, acme, created.ID))
			enrollments, err := env.DB.AutomationRun.Query().
				Where(automationrun.AutomationID(created.ID), automationrun.ContactID(fixtures.ContactAliceID)).Count(ctx)
			require.NoError(t, err)
			skipped, err := env.DB.AutomationSkippedEntry.Query().
				Where(automationskippedentry.AutomationID(created.ID), automationskippedentry.ContactID(fixtures.ContactAliceID)).
				Select(automationskippedentry.FieldReason).Strings(ctx)
			require.NoError(t, err)

			assert.Equal(t, c.enrollments, enrollments)
			assert.ElementsMatch(t, lo.Map(c.skipped, func(r automationskippedentry.Reason, _ int) string { return string(r) }), skipped)
		})
	}
}

// An agent reads which occurrence an Enrollment of a date Trigger was made for, its anniversary
// year (fixture Birthday greeting: Alice's for 2026), and none on an Enrollment an Event began.
func TestMCPEnrollmentsShowTheAnniversaryYearOfADateTrigger(t *testing.T) {
	env := testhelper.Setup(t)
	s := env.MCPClient(t, env.ScopedBearer(t, apitokens.ScopeEnrollmentsRead))

	dated := call(t, s, "enrollments_get", map[string]any{
		"automationId": testhelper.ID(fixtures.AutomationBirthdayGreetingID), "enrollmentId": testhelper.ID(fixtures.AutomationRunBirthdayGreetingAliceID),
	})
	evented := call(t, s, "enrollments_get", map[string]any{
		"automationId": testhelper.ID(fixtures.AutomationWelcomeSeriesID), "enrollmentId": testhelper.ID(fixtures.AutomationRunWelcomeCompletedID),
	})

	require.False(t, dated.IsError, text(t, dated))
	assert.Equal(t, map[string]any{"recurrence": "yearly", "year": float64(2026)}, decode(t, text(t, dated))["occurrence"])
	require.False(t, evented.IsError, text(t, evented))
	assert.NotContains(t, decode(t, text(t, evented)), "occurrence")
}
