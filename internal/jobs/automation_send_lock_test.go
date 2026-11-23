package jobs_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent/automationrun"
	"github.com/getsphericon/sphericon/internal/domain/orchestration/automations"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/jobs"
	"github.com/getsphericon/sphericon/internal/kernel/eventmatcher"
	"github.com/getsphericon/sphericon/internal/kernel/systemevent"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// An Event a write through a Connection without mcp:send published ends an Enrollment, and the
// automation.exited that ending publishes carries the mark: it enrolls no one in an Automation
// triggered by it, which an unmarked exit does (ADR 0132). Fixture: Payment recovery, which
// account_closed exits.
func TestAnEnrollmentEndedByASendLockedEventEnrollsNoOneElsewhere(t *testing.T) {
	for name, c := range map[string]struct {
		sendLocked bool
		want       int
	}{
		"send-locked": {sendLocked: true, want: 0},
		"unmarked":    {sendLocked: false, want: 1},
	} {
		t.Run(name, func(t *testing.T) {
			env := testhelper.Setup(t)
			ctx := t.Context()
			acme := env.DB.Scoped(fixtures.AcmeID)
			mod := testhelper.Module[*automations.Module](env)
			onExit, err := mod.Create(ctx, acme, automations.CreateInput{Name: "After an exit", Trigger: eventmatcher.Matcher{Action: systemevent.AutomationExited}})
			require.NoError(t, err)
			_, err = mod.Publish(ctx, acme, onExit.ID, testhelper.AutomationRevisionIn(ctx, t, acme, onExit.ID))
			require.NoError(t, err)
			steps := testhelper.Module[*automations.StepExecution](env)
			on := func(action string, sendLocked bool) {
				t.Helper()
				ev := automations.Event{Action: action, OccurredAt: time.Now(), SendLocked: sendLocked}
				require.NoError(t, steps.OnEvent(ctx, acme, automations.OnEventInput{ContactID: fixtures.ContactBobID, Event: ev}))
			}

			on("payment_failed", false)
			on("account_closed", c.sendLocked)
			env.PerformEnqueuedJobs(t, jobs.EvaluateTriggerArgs{})
			exited, err := env.DB.AutomationRun.Query().
				Where(automationrun.AutomationID(fixtures.AutomationPaymentRecoveryID), automationrun.ContactID(fixtures.ContactBobID)).
				Only(ctx)
			require.NoError(t, err)
			enrolled, err := env.DB.AutomationRun.Query().
				Where(automationrun.AutomationID(onExit.ID), automationrun.ContactID(fixtures.ContactBobID)).
				Count(ctx)
			require.NoError(t, err)

			assert.Equal(t, automationrun.StatusExited, exited.Status)
			assert.Equal(t, c.want, enrolled)
		})
	}
}
