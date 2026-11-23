package automations_test

import (
	"context"
	"slices"
	"testing"
	"time"

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
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// The Enrollment key (ADR 0165): a version keyed by a property of its Trigger Event runs one
// Enrollment per value of it, so each order gets its own series in parallel, and an exit or goal
// Event carrying the property acts on the Enrollment of its value only.

const (
	// orderPlaced starts the Automation keyedAutomation makes, orderCancelled exits it and
	// orderPaid reaches its goal.
	orderPlaced    = "order.placed"
	orderCancelled = "order.cancelled"
	orderPaid      = "order.paid"
)

// keyedAutomation creates and publishes an Automation keyed by order_id under the Entry policy:
// orderPlaced enrolls, orderCancelled exits, orderPaid reaches the goal, and its one Step, a
// month's wait, keeps an Enrollment active until then.
func keyedAutomation(t *testing.T, env *testhelper.TestEnv, policy automationversion.EntryPolicy) int64 {
	t.Helper()
	ctx := context.Background()
	s := env.DB.Scoped(fixtures.AcmeID)
	m := testhelper.Module[*automations.Module](env)
	created, err := m.Create(ctx, s, automations.CreateInput{
		Name: "Post-purchase", Trigger: eventmatcher.Matcher{Action: orderPlaced},
		Entry:      automations.Entry{Policy: policy, Key: new("order_id")},
		ExitEvents: []eventmatcher.Matcher{{Action: orderCancelled}},
		GoalEvent:  &eventmatcher.Matcher{Action: orderPaid},
		Steps:      new(automationstep.Root(automationstep.New(automationstep.Wait{Seconds: 30 * 24 * 3600}))),
	})
	require.NoError(t, err)
	_, err = m.Publish(ctx, s, created.ID, testhelper.AutomationRevisionIn(ctx, t, s, created.ID))
	require.NoError(t, err)
	return created.ID
}

// order hands the Automations a stored Event of the action for the Contact, occurred at, whose
// properties are props.
func order(t *testing.T, env *testhelper.TestEnv, contactID int64, action string, at time.Time, props map[string]any) {
	t.Helper()
	handStored(t, env, contactID, action, collected(contactID, action, at, props))
}

// keyed is the properties of an Event about the order.
func keyed(orderID any) map[string]any {
	return map[string]any{"order_id": orderID}
}

// statusByKey is the status of each of the Contact's Enrollments in the Automation, by its key
// value.
func statusByKey(t *testing.T, env *testhelper.TestEnv, automationID, contactID int64) map[string]automationrun.Status {
	t.Helper()
	return lo.SliceToMap(runsOf(t, env, automationID, contactID), func(r *ent.AutomationRun) (string, automationrun.Status) {
		return lo.FromPtr(r.KeyValue), r.Status
	})
}

// reasonsOf is the reasons the Contact's skipped entries into the Automation record, oldest first.
func reasonsOf(t *testing.T, env *testhelper.TestEnv, automationID, contactID int64) []automationskippedentry.Reason {
	t.Helper()
	skipped := skippedOf(t, env, automationID, contactID)
	reasons := lo.Map(skipped, func(e *ent.AutomationSkippedEntry, _ int) automationskippedentry.Reason { return e.Reason })
	slices.Reverse(reasons)
	return reasons
}

// A triggering Event without the key's property enrolls no one and records `no_enrollment_key`.
func TestAnEventWithoutTheKeyPropertyRecordsNoEnrollmentKey(t *testing.T) {
	env := testhelper.Setup(t)
	automationID := keyedAutomation(t, env, automationversion.EntryPolicyOnce)
	c := enrollable(t, env.DB.Scoped(fixtures.AcmeID), 0)

	order(t, env, c.ID, orderPlaced, moment(10), map[string]any{"total": 30})

	assert.Empty(t, runsOf(t, env, automationID, c.ID))
	assert.Equal(t, []automationskippedentry.Reason{automationskippedentry.ReasonNoEnrollmentKey}, reasonsOf(t, env, automationID, c.ID))
}

// Keyed by order_id, orders A and B make two active Enrollments, each keeping its value; a cancel
// for A ends A's only, and a cancel without order_id ends every one left.
func TestEachOrderRunsItsOwnEnrollmentAndACancelEndsItsOwnOnly(t *testing.T) {
	env := testhelper.Setup(t)
	automationID := keyedAutomation(t, env, automationversion.EntryPolicyOnce)
	c := enrollable(t, env.DB.Scoped(fixtures.AcmeID), 0)
	order(t, env, c.ID, orderPlaced, moment(50), keyed("A"))
	order(t, env, c.ID, orderPlaced, moment(40), keyed("B"))
	order(t, env, c.ID, orderPlaced, moment(35), keyed("C"))
	afterTheOrders := statusByKey(t, env, automationID, c.ID)

	order(t, env, c.ID, orderCancelled, moment(30), keyed("A"))
	afterCancellingA := statusByKey(t, env, automationID, c.ID)
	order(t, env, c.ID, orderCancelled, moment(20), map[string]any{"reason": "account closed"})

	active := automationrun.StatusActive
	assert.Equal(t, map[string]automationrun.Status{"A": active, "B": active, "C": active}, afterTheOrders)
	assert.Equal(t, map[string]automationrun.Status{"A": automationrun.StatusExited, "B": active, "C": active}, afterCancellingA)
	exited := automationrun.StatusExited
	assert.Equal(t, map[string]automationrun.Status{"A": exited, "B": exited, "C": exited}, statusByKey(t, env, automationID, c.ID))
	assert.Empty(t, skippedOf(t, env, automationID, c.ID))
}

// A goal Event carrying the key reaches the goal of the Enrollment of its value only.
func TestAGoalEventWithTheKeyReachesTheGoalOfItsOwnEnrollmentOnly(t *testing.T) {
	env := testhelper.Setup(t)
	automationID := keyedAutomation(t, env, automationversion.EntryPolicyOnce)
	c := enrollable(t, env.DB.Scoped(fixtures.AcmeID), 0)
	order(t, env, c.ID, orderPlaced, moment(50), keyed("A"))
	order(t, env, c.ID, orderPlaced, moment(40), keyed("B"))

	order(t, env, c.ID, orderPaid, moment(30), keyed("B"))

	assert.Equal(t, map[string]automationrun.Status{"A": automationrun.StatusActive, "B": automationrun.StatusGoalReached},
		statusByKey(t, env, automationID, c.ID))
}

// The module's Exit, which an unsubscribe from the Automation runs, ends every active Enrollment of
// the Contact in it, whatever its key value.
func TestExitEndsEveryKeyedEnrollmentOfTheContact(t *testing.T) {
	env := testhelper.Setup(t)
	automationID := keyedAutomation(t, env, automationversion.EntryPolicyOnce)
	c := enrollable(t, env.DB.Scoped(fixtures.AcmeID), 0)
	order(t, env, c.ID, orderPlaced, moment(50), keyed("A"))
	order(t, env, c.ID, orderPlaced, moment(40), keyed("B"))

	exited, err := env.ExitEnrollment(context.Background(), env.DB.Scoped(fixtures.AcmeID), automationID, c.ID)

	require.NoError(t, err)
	assert.True(t, exited)
	assert.Equal(t, map[string]automationrun.Status{"A": automationrun.StatusExited, "B": automationrun.StatusExited},
		statusByKey(t, env, automationID, c.ID))
}

// Under `once`, the same order never starts the series again, recording `once`, while a new order
// does: the Entry policy counts per key value.
func TestOnceCountsPerKeyValue(t *testing.T) {
	env := testhelper.Setup(t)
	automationID := keyedAutomation(t, env, automationversion.EntryPolicyOnce)
	c := enrollable(t, env.DB.Scoped(fixtures.AcmeID), 0)
	order(t, env, c.ID, orderPlaced, moment(50), keyed("A"))
	order(t, env, c.ID, orderCancelled, moment(40), keyed("A"))

	order(t, env, c.ID, orderPlaced, moment(30), keyed("A"))
	order(t, env, c.ID, orderPlaced, moment(20), keyed("C"))

	assert.Equal(t, map[string]automationrun.Status{"A": automationrun.StatusExited, "C": automationrun.StatusActive},
		statusByKey(t, env, automationID, c.ID))
	assert.Equal(t, []automationskippedentry.Reason{automationskippedentry.ReasonOnce}, reasonsOf(t, env, automationID, c.ID))
	runs := byOrdinal(t, env, automationID, c.ID)
	require.Len(t, runs, 2)
	assert.Equal(t, new("C"), runs[1].KeyValue)
	assert.Equal(t, 2, runs[1].Ordinal, "the ordinal counts the Contact's Enrollments of every key value")
}

// A second Event for an order whose Enrollment is active records `active_enrollment`, and under
// `again` the cooldown counts from the beginning of that order's previous Enrollment only.
func TestTheActiveEnrollmentAndTheCooldownCountPerKeyValue(t *testing.T) {
	env := testhelper.Setup(t)
	automationID := keyedAutomation(t, env, automationversion.EntryPolicyAgain)
	c := enrollable(t, env.DB.Scoped(fixtures.AcmeID), 0)
	order(t, env, c.ID, orderPlaced, moment(50), keyed("A"))
	order(t, env, c.ID, orderPlaced, moment(45), keyed("A"))
	order(t, env, c.ID, orderCancelled, moment(40), keyed("A"))

	order(t, env, c.ID, orderPlaced, moment(30), keyed("A"))
	order(t, env, c.ID, orderPlaced, moment(29), keyed("B"))

	assert.Equal(t, []automationskippedentry.Reason{automationskippedentry.ReasonActiveEnrollment, automationskippedentry.ReasonCooldown},
		reasonsOf(t, env, automationID, c.ID))
	assert.Equal(t, map[string]automationrun.Status{"A": automationrun.StatusExited, "B": automationrun.StatusActive},
		statusByKey(t, env, automationID, c.ID))
}

// A keyed Automation allows a Contact 10 active Enrollments: the 11th concurrent order records
// `enrollment_ceiling`.
func TestTheEleventhConcurrentOrderRecordsTheCeiling(t *testing.T) {
	env := testhelper.Setup(t)
	automationID := keyedAutomation(t, env, automationversion.EntryPolicyOnce)
	c := enrollable(t, env.DB.Scoped(fixtures.AcmeID), 0)
	for i := range 10 {
		order(t, env, c.ID, orderPlaced, moment(100-i), keyed(i))
	}

	order(t, env, c.ID, orderPlaced, moment(50), keyed("eleventh"))

	assert.Len(t, statusByKey(t, env, automationID, c.ID), 10)
	assert.Equal(t, []automationskippedentry.Reason{automationskippedentry.ReasonEnrollmentCeiling}, reasonsOf(t, env, automationID, c.ID))
}

// A keyed Automation allows a Contact 10 Enrollments begun in 24 hours, ended or not: the 11th
// records `enrollment_ceiling`, and once a day has passed since they began a new order enrolls.
func TestTheEleventhOrderBegunInADayRecordsTheCeiling(t *testing.T) {
	env := testhelper.Setup(t)
	automationID := keyedAutomation(t, env, automationversion.EntryPolicyOnce)
	c := enrollable(t, env.DB.Scoped(fixtures.AcmeID), 0)
	for i := range 10 {
		order(t, env, c.ID, orderPlaced, moment(200-i), keyed(i))
	}
	order(t, env, c.ID, orderCancelled, moment(150), nil)

	order(t, env, c.ID, orderPlaced, moment(100), keyed("eleventh"))
	order(t, env, c.ID, orderPlaced, moment(-25*60), keyed("next day"))

	assert.Equal(t, []automationskippedentry.Reason{automationskippedentry.ReasonEnrollmentCeiling}, reasonsOf(t, env, automationID, c.ID))
	byKey := statusByKey(t, env, automationID, c.ID)
	assert.Len(t, byKey, 11)
	assert.Equal(t, automationrun.StatusActive, byKey["next day"])
}

// The key's value is the property's string form, exact for a number however large: two orders
// whose ids a float would not tell apart run apart, and a cancel of one ends its own only.
func TestANumericKeyIsKeptExactly(t *testing.T) {
	env := testhelper.Setup(t)
	automationID := keyedAutomation(t, env, automationversion.EntryPolicyOnce)
	c := enrollable(t, env.DB.Scoped(fixtures.AcmeID), 0)
	order(t, env, c.ID, orderPlaced, moment(50), keyed(int64(9007199254740993)))
	order(t, env, c.ID, orderPlaced, moment(40), keyed(int64(9007199254740992)))

	order(t, env, c.ID, orderCancelled, moment(30), keyed(int64(9007199254740993)))

	assert.Equal(t, map[string]automationrun.Status{
		"9007199254740993": automationrun.StatusExited, "9007199254740992": automationrun.StatusActive,
	}, statusByKey(t, env, automationID, c.ID))
}

// A cancel of order A recorded after order B's Event occurred, but handled before B's entry, does
// not make B's Enrollment already ended: the check at entry narrows by the key too. A cancel
// without the key would.
func TestACancelOfAnotherOrderRecordedBeforeTheEntryDoesNotEndIt(t *testing.T) {
	env := testhelper.Setup(t)
	automationID := keyedAutomation(t, env, automationversion.EntryPolicyOnce)
	c := enrollable(t, env.DB.Scoped(fixtures.AcmeID), 0)
	order(t, env, c.ID, orderPlaced, moment(50), keyed("A"))
	order(t, env, c.ID, orderCancelled, moment(30), keyed("A"))

	order(t, env, c.ID, orderPlaced, moment(40), keyed("B"))
	order(t, env, c.ID, orderCancelled, moment(20), nil)
	order(t, env, c.ID, orderPlaced, moment(25), keyed("C"))

	assert.Equal(t, map[string]automationrun.Status{
		"A": automationrun.StatusExited, "B": automationrun.StatusExited, "C": automationrun.StatusExited,
	}, statusByKey(t, env, automationID, c.ID))
	runs := byOrdinal(t, env, automationID, c.ID)
	require.Len(t, runs, 3)
	assert.Less(t, moment(20).Sub(lo.FromPtr(runs[1].EndedAt)).Abs(), time.Minute,
		"B ends on the cancel without the key, not on A's")
}

// enrollOnStored runs a Rule Firing's Enroll Action for the Contact in the Automation on the Event
// last stored of the action.
func enrollOnStored(t *testing.T, env *testhelper.TestEnv, automationID, contactID, firingID int64, action string) automations.EnrollOnRuleResult {
	t.Helper()
	stored := env.StoredEvents(t, action)
	require.NotEmpty(t, stored)
	made, err := testhelper.Module[*automations.StepExecution](env).EnrollOnRule(context.Background(), env.DB.Scoped(fixtures.AcmeID),
		automations.EnrollOnRuleInput{AutomationID: automationID, ContactID: contactID, FiringID: firingID, SourceID: *stored[len(stored)-1].SourceID})
	require.NoError(t, err)
	return made
}

// A Rule's Enroll Action reads the key from the Event the Rule fired on, as the Trigger does: an
// Event with order_id makes the Enrollment of its value, one without records `no_enrollment_key`.
func TestARulesEnrollActionReadsTheKeyFromTheEventTheRuleFiredOn(t *testing.T) {
	env := testhelper.Setup(t)
	automationID := keyedAutomation(t, env, automationversion.EntryPolicyOnce)
	c := enrollable(t, env.DB.Scoped(fixtures.AcmeID), 0)
	require.NoError(t, env.Publisher().Publish(context.Background(), collected(c.ID, "order.reviewed", moment(30), keyed("R"))))
	made := enrollOnStored(t, env, automationID, c.ID, fixtures.RuleFiringDoneID, "order.reviewed")
	require.NoError(t, env.Publisher().Publish(context.Background(), collected(c.ID, "order.reviewed", moment(20), nil)))
	refused := enrollOnStored(t, env, automationID, c.ID, fixtures.RuleFiringPendingID, "order.reviewed")

	assert.Equal(t, automations.EnrollOnRuleResult{Enrolled: true}, made)
	assert.Equal(t, automations.EnrollOnRuleResult{Skipped: automationskippedentry.ReasonNoEnrollmentKey}, refused)
	assert.Equal(t, map[string]automationrun.Status{"R": automationrun.StatusActive}, statusByKey(t, env, automationID, c.ID))
}

// The Enrollment key is part of the definition: an edit of it alone writes the draft, an edit of
// the Trigger without it removes it, and a key that is no property name is refused.
func TestTheEnrollmentKeyIsWrittenWithTheTriggerAndCheckedForForm(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	s := env.DB.Scoped(fixtures.AcmeID)
	m := testhelper.Module[*automations.Module](env)
	automationID := keyedAutomation(t, env, automationversion.EntryPolicyOnce)
	revision := func() int64 { return testhelper.AutomationRevisionIn(ctx, t, s, automationID) }

	rekeyed, err := m.Update(ctx, s, automationID, automations.UpdateInput{
		Revision: revision(), Trigger: &eventmatcher.Matcher{Action: orderPlaced}, Entry: &automations.Entry{Key: new("invoice_id")},
	})
	require.NoError(t, err)
	live, draft := automations.LiveAndDraft(rekeyed)
	require.NotNil(t, draft)
	assert.Equal(t, new("order_id"), live.EnrollmentKey)
	assert.Equal(t, new("invoice_id"), draft.EnrollmentKey)

	unkeyed, err := m.Update(ctx, s, automationID, automations.UpdateInput{
		Revision: revision(), Trigger: &eventmatcher.Matcher{Action: orderPlaced}, Entry: &automations.Entry{},
	})
	require.NoError(t, err)
	_, draft = automations.LiveAndDraft(unkeyed)
	assert.Nil(t, draft.EnrollmentKey)

	_, refused := m.Update(ctx, s, automationID, automations.UpdateInput{
		Revision: revision(), Trigger: &eventmatcher.Matcher{Action: orderPlaced}, Entry: &automations.Entry{Key: new("")},
	})
	require.ErrorIs(t, refused, automations.ErrInvalidTrigger)
	assert.ErrorContains(t, refused, "property key is required")
}

// The report counts the entries skipped for want of the key and past the ceiling, and each order's
// Enrollment of the one Contact.
func TestTheReportCountsTheKeyedSkips(t *testing.T) {
	env := testhelper.Setup(t)
	automationID := keyedAutomation(t, env, automationversion.EntryPolicyOnce)
	c := enrollable(t, env.DB.Scoped(fixtures.AcmeID), 0)
	for i := range 10 {
		order(t, env, c.ID, orderPlaced, moment(100-i), keyed(i))
	}
	order(t, env, c.ID, orderPlaced, moment(50), keyed("eleventh"))
	order(t, env, c.ID, orderPlaced, moment(40), nil)

	report, err := testhelper.Module[*automations.Module](env).ReportOf(context.Background(), env.DB.Scoped(fixtures.AcmeID), automationID, automations.ReportPeriod7Days)

	require.NoError(t, err)
	current := report.Results.Current
	assert.Equal(t, 10, current.Entered)
	assert.Equal(t, 1, current.Contacts)
	assert.Equal(t, automations.SkippedCounts{NoEnrollmentKey: 1, EnrollmentCeiling: 1}, current.Skipped)
}
