package automations_test

import (
	"context"
	"errors"
	"testing"

	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/automationrun"
	"github.com/getsphericon/sphericon/ent/automationskippedentry"
	"github.com/getsphericon/sphericon/ent/automationversion"
	"github.com/getsphericon/sphericon/internal/domain/orchestration/automations"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/kernel/automationstep"
	"github.com/getsphericon/sphericon/internal/kernel/eventmatcher"
	"github.com/getsphericon/sphericon/internal/kernel/validation"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// The Entry policy (ADR 0127): under `again` a Contact enrolls once more after its Enrollment
// ended and the cooldown from that Enrollment's beginning passed, one active at a time, each
// Enrollment numbered by its ordinal; under `once` it enrolls once, ever.

const (
	// repeatTrigger starts the Automation againAutomation makes; repeatExit ends its Enrollment.
	repeatTrigger = "repeat_purchase"
	repeatExit    = "repeat_refunded"
)

// againAutomation creates and publishes an Automation under `again` with the default cooldown, an
// hour: repeatTrigger enrolls, repeatExit exits, and its one Step, a month's wait, keeps an
// Enrollment active until then.
func againAutomation(t *testing.T, env *testhelper.TestEnv) int64 {
	t.Helper()
	ctx := context.Background()
	s := env.DB.Scoped(fixtures.AcmeID)
	m := testhelper.Module[*automations.Module](env)
	created, err := m.Create(ctx, s, automations.CreateInput{
		Name: "Repeat purchase", Trigger: automations.EventTrigger{Matcher: eventmatcher.Matcher{Action: repeatTrigger}},
		Entry:      automations.Entry{Policy: automationversion.EntryPolicyAgain},
		ExitEvents: []eventmatcher.Matcher{{Action: repeatExit}},
		Steps:      new(automationstep.Root(automationstep.New(automationstep.Wait{Seconds: 30 * 24 * 3600}))),
	})
	require.NoError(t, err)
	_, err = m.Publish(ctx, s, created.ID, testhelper.AutomationRevisionIn(ctx, t, s, created.ID))
	require.NoError(t, err)
	return created.ID
}

// reasonsBySource is why each of the Contact's entries into the Automation was skipped, by the
// source id of the Event that tried.
func reasonsBySource(t *testing.T, env *testhelper.TestEnv, automationID, contactID int64) map[string]automationskippedentry.Reason {
	t.Helper()
	return lo.SliceToMap(skippedOf(t, env, automationID, contactID), func(e *ent.AutomationSkippedEntry) (string, automationskippedentry.Reason) {
		return lo.FromPtr(e.SourceID), e.Reason
	})
}

// byOrdinal is the Contact's Enrollments in the Automation, the first first.
func byOrdinal(t *testing.T, env *testhelper.TestEnv, automationID, contactID int64) []*ent.AutomationRun {
	t.Helper()
	runs, err := env.DB.AutomationRun.Query().
		Where(automationrun.AutomationID(automationID), automationrun.ContactID(contactID)).
		Order(ent.Asc(automationrun.FieldOrdinal)).
		All(context.Background())
	require.NoError(t, err)
	return runs
}

// Under `again`, once the first Enrollment ended and an hour passed since it began, the next
// triggering Event begins a second Enrollment, the Contact's second in the Automation.
func TestAgainEnrollsOnceMoreAfterTheEnrollmentEndedAndTheCooldownPassed(t *testing.T) {
	env := testhelper.Setup(t)
	automationID := againAutomation(t, env)
	c := enrollable(t, env.DB.Scoped(fixtures.AcmeID), 0)
	handSourcedEvent(t, env, c.ID, repeatTrigger, "evt-first", moment(300))
	handSourcedEvent(t, env, c.ID, repeatExit, "evt-refund", moment(280))

	second := moment(200)
	handSourcedEvent(t, env, c.ID, repeatTrigger, "evt-second", second)

	runs := byOrdinal(t, env, automationID, c.ID)
	require.Len(t, runs, 2)
	assert.Equal(t, 1, runs[0].Ordinal)
	assert.Equal(t, automationrun.StatusExited, runs[0].Status)
	assert.Equal(t, 2, runs[1].Ordinal)
	assert.Equal(t, automationrun.StatusActive, runs[1].Status)
	assert.Equal(t, new("evt-second"), runs[1].TriggerSourceID)
	assert.True(t, second.Equal(runs[1].BeganAt))
	assert.Empty(t, skippedOf(t, env, automationID, c.ID))
}

// Under `again`, an Event less than the cooldown after the previous Enrollment began records
// `cooldown`, though that Enrollment ended; one while it is active records `active_enrollment`.
func TestAgainRecordsTheCooldownAndTheActiveEnrollment(t *testing.T) {
	env := testhelper.Setup(t)
	automationID := againAutomation(t, env)
	c := enrollable(t, env.DB.Scoped(fixtures.AcmeID), 0)
	handSourcedEvent(t, env, c.ID, repeatTrigger, "evt-first", moment(300))
	handSourcedEvent(t, env, c.ID, repeatTrigger, "evt-while-active", moment(290))
	handSourcedEvent(t, env, c.ID, repeatExit, "evt-refund", moment(280))

	handSourcedEvent(t, env, c.ID, repeatTrigger, "evt-within-the-cooldown", moment(250))

	reasons := reasonsBySource(t, env, automationID, c.ID)
	assert.Equal(t, map[string]automationskippedentry.Reason{
		"evt-while-active":        automationskippedentry.ReasonActiveEnrollment,
		"evt-within-the-cooldown": automationskippedentry.ReasonCooldown,
	}, reasons)
	runs := runsOf(t, env, automationID, c.ID)
	assert.Len(t, runs, 1)
}

// A Contact has one active Enrollment in an Automation: an Event that occurred before the active
// Enrollment began, handled while it runs, records `active_enrollment` rather than starting a
// second beside it, however long before it occurred.
func TestAnEventOlderThanTheActiveEnrollmentRecordsActiveEnrollment(t *testing.T) {
	env := testhelper.Setup(t)
	automationID := againAutomation(t, env)
	c := enrollable(t, env.DB.Scoped(fixtures.AcmeID), 0)
	handSourcedEvent(t, env, c.ID, repeatTrigger, "evt-first", moment(30))

	handSourcedEvent(t, env, c.ID, repeatTrigger, "evt-backdated", moment(600))

	reasons := reasonsBySource(t, env, automationID, c.ID)
	assert.Equal(t, map[string]automationskippedentry.Reason{"evt-backdated": automationskippedentry.ReasonActiveEnrollment}, reasons)
	runs := runsOf(t, env, automationID, c.ID)
	assert.Len(t, runs, 1)
}

// enrollOnFiringResult runs a Rule Firing's Enroll Action for the Contact in the Automation, on an
// Event no longer stored, so the Enrollment would begin now.
func enrollOnFiringResult(t *testing.T, env *testhelper.TestEnv, automationID, contactID, firingID int64) automations.EnrollOnRuleResult {
	t.Helper()
	made, err := testhelper.Module[*automations.StepExecution](env).EnrollOnRule(context.Background(), env.DB.Scoped(fixtures.AcmeID),
		automations.EnrollOnRuleInput{AutomationID: automationID, ContactID: contactID, FiringID: firingID, SourceID: "evt-not-stored"})
	require.NoError(t, err)
	return made
}

// A Rule's Enroll Action obeys `again` as the Trigger does: after the Enrollment the Trigger made
// ended and the cooldown passed it makes the second, and while that one is active the next Firing
// is skipped as `active_enrollment`, which the result names.
func TestARulesEnrollActionEnrollsAgainUnderAgain(t *testing.T) {
	env := testhelper.Setup(t)
	automationID := againAutomation(t, env)
	c := enrollable(t, env.DB.Scoped(fixtures.AcmeID), 0)
	handSourcedEvent(t, env, c.ID, repeatTrigger, "evt-first", moment(300))
	handSourcedEvent(t, env, c.ID, repeatExit, "evt-refund", moment(280))

	made := enrollOnFiringResult(t, env, automationID, c.ID, fixtures.RuleFiringDoneID)
	refused := enrollOnFiringResult(t, env, automationID, c.ID, fixtures.RuleFiringPendingID)

	assert.Equal(t, automations.EnrollOnRuleResult{Enrolled: true}, made)
	assert.Equal(t, automations.EnrollOnRuleResult{Skipped: automationskippedentry.ReasonActiveEnrollment}, refused)
	runs := byOrdinal(t, env, automationID, c.ID)
	require.Len(t, runs, 2)
	assert.Equal(t, new(int64(fixtures.RuleFiringDoneID)), runs[1].FiringID)
	assert.Equal(t, 2, runs[1].Ordinal)
}

// A Rule's Enroll Action within the cooldown of the previous Enrollment's beginning is skipped as
// `cooldown`, and the result names it.
func TestARulesEnrollActionWithinTheCooldownRecordsCooldown(t *testing.T) {
	env := testhelper.Setup(t)
	automationID := againAutomation(t, env)
	c := enrollable(t, env.DB.Scoped(fixtures.AcmeID), 0)
	handSourcedEvent(t, env, c.ID, repeatTrigger, "evt-first", moment(30))
	handSourcedEvent(t, env, c.ID, repeatExit, "evt-refund", moment(20))

	refused := enrollOnFiringResult(t, env, automationID, c.ID, fixtures.RuleFiringDoneID)

	assert.Equal(t, automations.EnrollOnRuleResult{Skipped: automationskippedentry.ReasonCooldown}, refused)
	skipped := skippedOf(t, env, automationID, c.ID)
	require.Len(t, skipped, 1)
	assert.Equal(t, new(int64(fixtures.RuleFiringDoneID)), skipped[0].FiringID)
	assert.Equal(t, automationskippedentry.ReasonCooldown, skipped[0].Reason)
}

// With several Enrollments of one Contact in the Automation, an exit Event ends the active one
// only, and the module's Exit too: the ended ones keep how and when they ended.
func TestTheEndingsActOnTheActiveEnrollmentAmongSeveral(t *testing.T) {
	env := testhelper.Setup(t)
	automationID := againAutomation(t, env)
	c := enrollable(t, env.DB.Scoped(fixtures.AcmeID), 0)
	firstEnd := moment(280)
	handSourcedEvent(t, env, c.ID, repeatTrigger, "evt-first", moment(300))
	handSourcedEvent(t, env, c.ID, repeatExit, "evt-refund", firstEnd)
	handSourcedEvent(t, env, c.ID, repeatTrigger, "evt-second", moment(200))
	handSourcedEvent(t, env, c.ID, repeatExit, "evt-second-refund", moment(150))
	handSourcedEvent(t, env, c.ID, repeatTrigger, "evt-third", moment(100))

	exited, err := env.ExitEnrollment(context.Background(), env.DB.Scoped(fixtures.AcmeID), automationID, c.ID)

	require.NoError(t, err)
	assert.True(t, exited)
	runs := byOrdinal(t, env, automationID, c.ID)
	require.Len(t, runs, 3)
	statuses := lo.Map(runs, func(r *ent.AutomationRun, _ int) automationrun.Status { return r.Status })
	assert.Equal(t, []automationrun.Status{automationrun.StatusExited, automationrun.StatusExited, automationrun.StatusExited}, statuses)
	assert.True(t, firstEnd.Equal(lo.FromPtr(runs[0].EndedAt)), "the first keeps the moment its exit Event occurred")
	again, err := env.ExitEnrollment(context.Background(), env.DB.Scoped(fixtures.AcmeID), automationID, c.ID)
	require.NoError(t, err)
	assert.False(t, again, "no Enrollment is active any more")
}

// The report counts each Enrollment of a Contact that enrolled twice, the one Contact among them,
// and the entry the cooldown refused.
func TestTheReportCountsEveryEnrollmentOfAContactAndTheCooldown(t *testing.T) {
	env := testhelper.Setup(t)
	automationID := againAutomation(t, env)
	c := enrollable(t, env.DB.Scoped(fixtures.AcmeID), 0)
	handSourcedEvent(t, env, c.ID, repeatTrigger, "evt-first", moment(300))
	handSourcedEvent(t, env, c.ID, repeatExit, "evt-refund", moment(280))
	handSourcedEvent(t, env, c.ID, repeatTrigger, "evt-within-the-cooldown", moment(270))
	handSourcedEvent(t, env, c.ID, repeatTrigger, "evt-second", moment(200))

	report, err := testhelper.Module[*automations.Module](env).ReportOf(context.Background(), env.DB.Scoped(fixtures.AcmeID), automationID, automations.ReportPeriod7Days)

	require.NoError(t, err)
	current := report.Results.Current
	assert.Equal(t, 2, current.Entered)
	assert.Equal(t, 1, current.Exited)
	assert.Equal(t, 1, current.Contacts)
	assert.Equal(t, automations.SkippedCounts{Cooldown: 1}, current.Skipped)
}

// A version's Entry policy defaults to `once` with an hour's cooldown, and a cooldown under an hour
// is refused on save, naming the field.
func TestTheEntryPolicyDefaultsToOnceAndACooldownUnderAnHourIsRefused(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	s := env.DB.Scoped(fixtures.AcmeID)
	m := testhelper.Module[*automations.Module](env)

	plain, err := m.Create(ctx, s, automations.CreateInput{Name: "Plain", Trigger: automations.EventTrigger{Matcher: eventmatcher.Matcher{Action: repeatTrigger}}})
	require.NoError(t, err)
	_, refused := m.Create(ctx, s, automations.CreateInput{
		Name: "Too often", Trigger: automations.EventTrigger{Matcher: eventmatcher.Matcher{Action: repeatTrigger}},
		Entry: automations.Entry{Policy: automationversion.EntryPolicyAgain, CooldownSeconds: new(1800)},
	})

	_, draft := automations.LiveAndDraft(plain)
	require.NotNil(t, draft)
	assert.Equal(t, automationversion.EntryPolicyOnce, draft.EntryPolicy)
	assert.Equal(t, 3600, draft.CooldownSeconds)
	require.ErrorIs(t, refused, validation.ErrTooSmall)
	field, ok := errors.AsType[*validation.RefusalError](refused)
	require.True(t, ok, "a cooldown under an hour is a refusal: %v", refused)
	assert.Equal(t, "cooldown", field.Field())
}

// A change of the Entry policy alone is a change of the definition: it writes the draft.
func TestAnEditOfTheEntryPolicyAloneWritesTheDraft(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	s := env.DB.Scoped(fixtures.AcmeID)
	m := testhelper.Module[*automations.Module](env)
	automationID := againAutomation(t, env)

	edited, err := m.Update(ctx, s, automationID, automations.UpdateInput{
		Revision: testhelper.AutomationRevisionIn(ctx, t, s, automationID),
		Trigger:  automations.EventTrigger{Matcher: eventmatcher.Matcher{Action: repeatTrigger}},
		Entry:    &automations.Entry{Policy: automationversion.EntryPolicyAgain, CooldownSeconds: new(7200)},
	})

	require.NoError(t, err)
	live, draft := automations.LiveAndDraft(edited)
	require.NotNil(t, draft)
	assert.Equal(t, 3600, live.CooldownSeconds)
	assert.Equal(t, 7200, draft.CooldownSeconds)
	assert.Equal(t, automationversion.EntryPolicyAgain, draft.EntryPolicy)
}

// The cooldown counts from the beginning of the previous Enrollment, not its end: an Event a minute
// after a long Enrollment ended, hours after it began, enrolls.
func TestTheCooldownCountsFromTheBeginningOfThePreviousEnrollment(t *testing.T) {
	env := testhelper.Setup(t)
	automationID := againAutomation(t, env)
	c := enrollable(t, env.DB.Scoped(fixtures.AcmeID), 0)
	handSourcedEvent(t, env, c.ID, repeatTrigger, "evt-first", moment(300))
	handSourcedEvent(t, env, c.ID, repeatExit, "evt-refund", moment(100))

	handSourcedEvent(t, env, c.ID, repeatTrigger, "evt-second", moment(99))

	runs := byOrdinal(t, env, automationID, c.ID)
	require.Len(t, runs, 2)
	assert.Equal(t, new("evt-second"), runs[1].TriggerSourceID)
	assert.Empty(t, skippedOf(t, env, automationID, c.ID))
}

// Under `again`, an Event handled again after the Enrollment it made ended and the next began is
// no new entry: it makes nothing and records no skip, and so is a Firing run again.
func TestAnEntryHandledAgainUnderAgainRecordsNothing(t *testing.T) {
	env := testhelper.Setup(t)
	automationID := againAutomation(t, env)
	c := enrollable(t, env.DB.Scoped(fixtures.AcmeID), 0)
	first := moment(300)
	handSourcedEvent(t, env, c.ID, repeatTrigger, "evt-first", first)
	handSourcedEvent(t, env, c.ID, repeatExit, "evt-refund", moment(280))
	made := enrollOnFiringResult(t, env, automationID, c.ID, fixtures.RuleFiringDoneID)

	handSourcedEvent(t, env, c.ID, repeatTrigger, "evt-first", first)
	again := enrollOnFiringResult(t, env, automationID, c.ID, fixtures.RuleFiringDoneID)

	assert.True(t, made.Enrolled)
	assert.Equal(t, automations.EnrollOnRuleResult{}, again)
	runs := runsOf(t, env, automationID, c.ID)
	assert.Len(t, runs, 2)
	assert.Empty(t, skippedOf(t, env, automationID, c.ID))
}
