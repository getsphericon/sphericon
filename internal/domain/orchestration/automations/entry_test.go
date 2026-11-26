package automations_test

import (
	"testing"
	"time"

	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/automationrun"
	"github.com/getsphericon/sphericon/ent/automationskippedentry"
	"github.com/getsphericon/sphericon/internal/domain/orchestration/automations"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/kernel/pagination"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// handSourcedEvent hands the Event of the Contact stored under sourceID, with the action, occurred
// at, to the Automations, as its trigger job does.
func handSourcedEvent(t *testing.T, env *testhelper.TestEnv, contactID int64, action, sourceID string, at time.Time) {
	t.Helper()
	in := automations.OnEventInput{ContactID: contactID, Event: automations.Event{Action: action, SourceID: sourceID, OccurredAt: at}}
	require.NoError(t, testhelper.Module[*automations.StepExecution](env).OnEvent(t.Context(), env.DB.Scoped(fixtures.AcmeID), in))
}

// skippedOf is the Contact's skipped entries into the Automation, read through the module.
func skippedOf(t *testing.T, env *testhelper.TestEnv, automationID, contactID int64) []*ent.AutomationSkippedEntry {
	t.Helper()
	page, err := testhelper.Module[*automations.Module](env).SkippedEntries(t.Context(), env.DB.Scoped(fixtures.AcmeID),
		pagination.Params{Page: 1, PageSize: 100}, automations.SkippedEntriesFilter{AutomationID: automationID, ContactID: contactID})
	require.NoError(t, err)
	return page.Items
}

// moment is a time minutes ago, kept to the microsecond as Postgres keeps it.
func moment(minutesAgo int) time.Time {
	return time.Now().Add(-time.Duration(minutesAgo) * time.Minute).Truncate(time.Microsecond)
}

// Under `once`, a second triggering Event after the first Enrollment ended enrolls no one and
// records `once`, with the Event that tried and its moment.
func TestASecondTriggerAfterTheEnrollmentEndedRecordsOnce(t *testing.T) {
	env := testhelper.Setup(t)
	c := enrollable(t, env.DB.Scoped(fixtures.AcmeID), 0)
	handSourcedEvent(t, env, c.ID, "payment_failed", "evt-first", moment(30))
	handSourcedEvent(t, env, c.ID, recoveryExit, "evt-exit", moment(20))

	second := moment(10)
	handSourcedEvent(t, env, c.ID, "payment_failed", "evt-second", second)

	runs := runsOf(t, env, fixtures.AutomationPaymentRecoveryID, c.ID)
	require.Len(t, runs, 1)
	assert.Equal(t, automationrun.StatusExited, runs[0].Status)
	skipped := skippedOf(t, env, fixtures.AutomationPaymentRecoveryID, c.ID)
	require.Len(t, skipped, 1)
	assert.Equal(t, automationskippedentry.ReasonOnce, skipped[0].Reason)
	assert.Equal(t, new("evt-second"), skipped[0].SourceID)
	assert.True(t, second.Equal(skipped[0].OccurredAt))
}

// A triggering Event that occurred while an Enrollment was active records `active_enrollment`, even
// when it is handled after that Enrollment ended: it wins over `once`. Active means begun at the
// Event's moment or before and not yet ended then.
func TestATriggerThatOccurredWhileAnEnrollmentWasActiveRecordsActiveEnrollment(t *testing.T) {
	env := testhelper.Setup(t)
	c := enrollable(t, env.DB.Scoped(fixtures.AcmeID), 0)
	began, ended := moment(30), moment(10)
	handSourcedEvent(t, env, c.ID, "payment_failed", "evt-first", began)
	handSourcedEvent(t, env, c.ID, recoveryExit, "evt-exit", ended)

	handSourcedEvent(t, env, c.ID, "payment_failed", "evt-during", moment(20))
	handSourcedEvent(t, env, c.ID, "payment_failed", "evt-at-the-beginning", began)
	handSourcedEvent(t, env, c.ID, "payment_failed", "evt-at-the-end", ended)
	handSourcedEvent(t, env, c.ID, "payment_failed", "evt-before", moment(40))

	skipped := skippedOf(t, env, fixtures.AutomationPaymentRecoveryID, c.ID)
	reasons := lo.SliceToMap(skipped, func(e *ent.AutomationSkippedEntry) (string, automationskippedentry.Reason) {
		return lo.FromPtr(e.SourceID), e.Reason
	})
	assert.Equal(t, map[string]automationskippedentry.Reason{
		"evt-during":           automationskippedentry.ReasonActiveEnrollment,
		"evt-at-the-beginning": automationskippedentry.ReasonActiveEnrollment,
		"evt-at-the-end":       automationskippedentry.ReasonOnce,
		"evt-before":           automationskippedentry.ReasonOnce,
	}, reasons)
	runs := runsOf(t, env, fixtures.AutomationPaymentRecoveryID, c.ID)
	assert.Len(t, runs, 1)
}

// The same Event handled twice makes one Enrollment and no skipped entry, and an Event skipped once
// is not recorded again when it is handled again.
func TestTheSameEventHandledTwiceMakesOneEnrollmentAndAtMostOneSkippedEntry(t *testing.T) {
	env := testhelper.Setup(t)
	c := enrollable(t, env.DB.Scoped(fixtures.AcmeID), 0)
	first, second := moment(20), moment(10)

	handSourcedEvent(t, env, c.ID, "payment_failed", "evt-first", first)
	handSourcedEvent(t, env, c.ID, "payment_failed", "evt-first", first)
	handSourcedEvent(t, env, c.ID, "payment_failed", "evt-second", second)
	handSourcedEvent(t, env, c.ID, "payment_failed", "evt-second", second)

	runs := runsOf(t, env, fixtures.AutomationPaymentRecoveryID, c.ID)
	require.Len(t, runs, 1)
	assert.Equal(t, new("evt-first"), runs[0].TriggerSourceID)
	skipped := skippedOf(t, env, fixtures.AutomationPaymentRecoveryID, c.ID)
	require.Len(t, skipped, 1)
	assert.Equal(t, new("evt-second"), skipped[0].SourceID)
	assert.Equal(t, automationskippedentry.ReasonActiveEnrollment, skipped[0].Reason)
}

// enrollOnFiring runs a Rule Firing's Enroll Action for the Contact in the Automation.
func enrollOnFiring(t *testing.T, env *testhelper.TestEnv, automationID, contactID, firingID int64) bool {
	t.Helper()
	made, err := testhelper.Module[*automations.StepExecution](env).EnrollOnRule(t.Context(), env.DB.Scoped(fixtures.AcmeID),
		automations.EnrollOnRuleInput{AutomationID: automationID, ContactID: contactID, FiringID: firingID, SourceID: "evt-not-stored"})
	require.NoError(t, err)
	return made.Enrolled
}

// A Rule Firing's Enroll Action run twice makes one Enrollment, and a Firing refused by the entry
// checks records one skipped entry however often it runs.
func TestTheSameFiringRunTwiceMakesOneEnrollmentAndAtMostOneSkippedEntry(t *testing.T) {
	env := testhelper.Setup(t)
	c := enrollable(t, env.DB.Scoped(fixtures.AcmeID), 0)

	enrolled := enrollOnFiring(t, env, fixtures.AutomationPaymentRecoveryID, c.ID, fixtures.RuleFiringDoneID)
	again := enrollOnFiring(t, env, fixtures.AutomationPaymentRecoveryID, c.ID, fixtures.RuleFiringDoneID)
	refused := enrollOnFiring(t, env, fixtures.AutomationPaymentRecoveryID, c.ID, fixtures.RuleFiringPendingID)
	refusedAgain := enrollOnFiring(t, env, fixtures.AutomationPaymentRecoveryID, c.ID, fixtures.RuleFiringPendingID)

	assert.True(t, enrolled)
	assert.False(t, again)
	assert.False(t, refused)
	assert.False(t, refusedAgain)
	runs := runsOf(t, env, fixtures.AutomationPaymentRecoveryID, c.ID)
	require.Len(t, runs, 1)
	assert.Equal(t, new(int64(fixtures.RuleFiringDoneID)), runs[0].FiringID)
	skipped := skippedOf(t, env, fixtures.AutomationPaymentRecoveryID, c.ID)
	require.Len(t, skipped, 1)
	assert.Equal(t, new(int64(fixtures.RuleFiringPendingID)), skipped[0].FiringID)
	assert.Nil(t, skipped[0].SourceID)
	assert.Equal(t, automationskippedentry.ReasonActiveEnrollment, skipped[0].Reason)
}
