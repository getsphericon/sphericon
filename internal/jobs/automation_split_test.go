package jobs_test

import (
	"testing"
	"time"

	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/automationrun"
	"github.com/getsphericon/sphericon/ent/automationsteprecord"
	"github.com/getsphericon/sphericon/ent/automationversion"
	"github.com/getsphericon/sphericon/ent/outboundmessage"
	"github.com/getsphericon/sphericon/internal/domain/contacts/contacts"
	"github.com/getsphericon/sphericon/internal/domain/orchestration/automations"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/jobs"
	"github.com/getsphericon/sphericon/internal/kernel/automationstep"
	"github.com/getsphericon/sphericon/internal/kernel/datetrigger"
	"github.com/getsphericon/sphericon/internal/kernel/eventmatcher"
	"github.com/getsphericon/sphericon/internal/kernel/localcalendar"
	"github.com/getsphericon/sphericon/internal/kernel/patch"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// splitTrigger is the action that enrolls the Automations of these tests.
const splitTrigger = "split.entered"

// The Split of these tests and its paths. Its id is pinned: a Contact's draw at a Split is a hash
// of the Contact's id, the Split's id and the Enrollment's ordinal (computed apart from the code
// with shasum), and at this id Alice's first Enrollment draws 39, Bob's first 96 and Alice's
// second 91. Of a 50/50 Split, 0 to 49 take the first path, the variant, and 50 to 99 the second,
// the holdout.
const (
	pinnedSplitID = automationstep.ID("5c1d0e2f-8a4b-4c3d-9e2f-000000000006")
	variantPathID = automationstep.ID("5c1d0e2f-8a4b-4c3d-9e2f-000000000101")
	holdoutPathID = automationstep.ID("5c1d0e2f-8a4b-4c3d-9e2f-000000000102")
)

// fiftyFifty is a Split of the pinned id whose first path, of weight 50, sends an email and whose
// second, of weight 50, is an empty holdout.
func fiftyFifty() automationstep.Step {
	return automationstep.Step{ID: pinnedSplitID, Payload: automationstep.Split{Paths: []automationstep.SplitPath{
		{ID: variantPathID, Weight: 50, Steps: []automationstep.Step{
			automationstep.New(automationstep.Email{Subject: "Try this", Body: "<p>A variant.</p>"}),
		}},
		{ID: holdoutPathID, Weight: 50, Steps: []automationstep.Step{}},
	}}}
}

// publishedSplitting makes an Automation of Acme started by splitTrigger that enrolls a Contact
// again once its Enrollment ended, with the root path's Steps, and publishes it.
func publishedSplitting(t *testing.T, env *testhelper.TestEnv, steps ...automationstep.Step) *ent.Automation {
	t.Helper()
	ctx := t.Context()
	s := env.DB.Scoped(fixtures.AcmeID)
	m := testhelper.Module[*automations.Module](env)
	root := automationstep.Root(steps...)
	a, err := m.Create(ctx, s, automations.CreateInput{
		Name: "Splitting", Trigger: automations.Trigger{Event: eventmatcher.Matcher{Action: splitTrigger}},
		Entry: automations.Entry{Policy: automationversion.EntryPolicyAgain}, Steps: &root,
	})
	require.NoError(t, err)
	a, err = m.Publish(ctx, s, a.ID, testhelper.AutomationRevisionIn(ctx, t, s, a.ID))
	require.NoError(t, err)
	return a
}

// enterSplitAt hands the Contact's splitTrigger, occurred at, to the Automations of Acme.
func enterSplitAt(t *testing.T, env *testhelper.TestEnv, contactID int64, at time.Time) {
	t.Helper()
	in := automations.OnEventInput{ContactID: contactID, Event: automations.Event{Action: splitTrigger, OccurredAt: at}}
	require.NoError(t, testhelper.Module[*automations.StepExecution](env).OnEvent(t.Context(), env.DB.Scoped(fixtures.AcmeID), in))
}

// onlyEnrollment is the one Enrollment of the Contact in the Automation.
func onlyEnrollment(t *testing.T, env *testhelper.TestEnv, automationID, contactID int64) int64 {
	t.Helper()
	runIDs := enrollmentsOf(t, env, automationID, contactID)
	require.Len(t, runIDs, 1)
	return runIDs[0]
}

// sentBy is the subjects of the email the Enrollment sent.
func sentBy(t *testing.T, env *testhelper.TestEnv, runID int64) []string {
	t.Helper()
	msgs := env.DB.OutboundMessage.Query().Where(outboundmessage.AutomationRunID(runID)).AllX(t.Context())
	return lo.Map(msgs, func(m *ent.OutboundMessage, _ int) string { return lo.FromPtr(m.Subject) })
}

// A Split 50/50 with an empty path sends nothing to the holdout: Alice draws the variant and is sent
// its email, Bob draws the holdout and is sent nothing; both go on to the Step after the Split, and
// the Split's record keeps the path each took.
func TestASplitSendsNothingToItsHoldout(t *testing.T) {
	env := testhelper.Setup(t)
	after := tagStep("after-split")
	auto := publishedSplitting(t, env, fiftyFifty(), after)
	enterSplitAt(t, env, fixtures.ContactAliceID, time.Now())
	enterSplitAt(t, env, fixtures.ContactBobID, time.Now())
	alice := onlyEnrollment(t, env, auto.ID, fixtures.ContactAliceID)
	bob := onlyEnrollment(t, env, auto.ID, fixtures.ContactBobID)

	env.RunQueuedSteps(t, stepperOf(env, resolvingTo(&fakeSender{})))

	aliceSplit := recordOf(t, env, alice, pinnedSplitID)
	bobSplit := recordOf(t, env, bob, pinnedSplitID)
	assert.Equal(t, automationsteprecord.OutcomePath, lo.FromPtr(bobSplit.Outcome))
	assert.Equal(t, variantPathID, tookPath(aliceSplit))
	assert.Equal(t, holdoutPathID, tookPath(bobSplit))
	assert.Nil(t, bobSplit.PathOutcomes, "a Split tests no Condition")
	assert.Equal(t, []string{"Try this"}, sentBy(t, env, alice))
	assert.Empty(t, sentBy(t, env, bob), "the holdout is sent nothing")
	assert.NotNil(t, recordOf(t, env, alice, after.ID).LeftAt)
	assert.NotNil(t, recordOf(t, env, bob, after.ID).LeftAt, "the holdout goes on to the Step after the Split")
	bobRun := env.DB.AutomationRun.GetX(t.Context(), bob)
	assert.Equal(t, automationrun.StatusCompleted, bobRun.Status)
}

// The path is a draw of the Contact, the Split and the Enrollment's ordinal, with no random state:
// the same Enrollment lands on the same path wherever the Split runs for it (here, a second
// Automation holding the same Split), and a new Enrollment of the Contact draws again. Alice's first
// Enrollment takes the variant in both Automations; her second takes the holdout.
func TestASplitDrawsOncePerEnrollmentAndAgainForANewOne(t *testing.T) {
	env := testhelper.Setup(t)
	auto := publishedSplitting(t, env, fiftyFifty())
	twin := publishedSplitting(t, env, fiftyFifty())
	// Its Steps hold no wait: they run on the wall clock, so the first Enrollment ends before the
	// second begins.
	stepper := stepperAt(env, newMod(env, resolvingTo(&fakeSender{}), time.Now), time.Now)
	enterSplitAt(t, env, fixtures.ContactAliceID, time.Now().Add(-2*time.Hour))
	env.RunQueuedSteps(t, stepper)

	enterSplitAt(t, env, fixtures.ContactAliceID, time.Now())
	env.RunQueuedSteps(t, stepper)

	runs := byOrdinalOf(t, env, auto.ID, fixtures.ContactAliceID)
	twinRuns := byOrdinalOf(t, env, twin.ID, fixtures.ContactAliceID)
	require.Len(t, runs, 2)
	require.Len(t, twinRuns, 2)
	first := tookPath(recordOf(t, env, runs[0].ID, pinnedSplitID))
	twinFirst := tookPath(recordOf(t, env, twinRuns[0].ID, pinnedSplitID))
	second := tookPath(recordOf(t, env, runs[1].ID, pinnedSplitID))
	assert.Equal(t, variantPathID, first)
	assert.Equal(t, first, twinFirst, "the same Enrollment's draw lands on the same path")
	assert.Equal(t, holdoutPathID, second, "a new Enrollment draws again")
	assert.Equal(t, []string{"Try this"}, sentBy(t, env, runs[0].ID))
	assert.Empty(t, sentBy(t, env, runs[1].ID))
}

// A date-triggered Split case (ADR 0126): an Instance whose clock the test moves, Alice born on 14
// March in Acme's zone (UTC, as she has none), and the Step execution that runs her Steps on that
// clock.
type datedSplit struct {
	t       *testing.T
	env     *testhelper.TestEnv
	clock   *testhelper.Clock
	stepper *automations.StepExecution
}

func newDatedSplit(t *testing.T, at time.Time) *datedSplit {
	t.Helper()
	clock := testhelper.NewClock(at)
	env := testhelper.Setup(t, testhelper.WithClock(clock.Now))
	ctx := t.Context()
	alice, err := env.DB.Contact.Get(ctx, fixtures.ContactAliceID)
	require.NoError(t, err)
	_, err = testhelper.Module[*contacts.Module](env).Update(ctx, env.DB.Scoped(fixtures.AcmeID), alice.ID, contacts.UpdateInput{
		Revision: alice.Revision, Birthday: patch.Set(time.Date(1990, time.March, 14, 0, 0, 0, 0, time.UTC)),
	})
	require.NoError(t, err)
	stepper := stepperAt(env, newMod(env, resolvingTo(&fakeSender{}), clock.Now), clock.Now)
	return &datedSplit{t: t, env: env, clock: clock, stepper: stepper}
}

// publish makes an Automation of Acme on the date with the root path's Steps and publishes it at the
// clock's now.
func (c *datedSplit) publish(d datetrigger.Date, steps ...automationstep.Step) int64 {
	c.t.Helper()
	ctx := c.t.Context()
	acme := c.env.DB.Scoped(fixtures.AcmeID)
	m := testhelper.Module[*automations.Module](c.env)
	root := automationstep.Root(steps...)
	created, err := m.Create(ctx, acme, automations.CreateInput{Name: "Birthday split", Trigger: automations.Trigger{Date: &d}, Steps: &root})
	require.NoError(c.t, err)
	_, err = m.Publish(ctx, acme, created.ID, testhelper.AutomationRevisionIn(ctx, c.t, acme, created.ID))
	require.NoError(c.t, err)
	return created.ID
}

// tickAt moves the clock to the moment, works the date scheduler's tick and the decisions it queues,
// then every Step they queued.
func (c *datedSplit) tickAt(now time.Time) {
	c.t.Helper()
	c.clock.Set(now)
	c.env.EnqueueJob(c.t, jobs.DateTickArgs{})
	c.env.PerformEnqueuedJobs(c.t, jobs.DateTickArgs{}, jobs.DecideOccurrencesArgs{})
	c.env.RunQueuedSteps(c.t, c.stepper)
}

// pathsOf is the path Alice's Enrollments in the Automation took at the Split, the first first.
func (c *datedSplit) pathsOf(automationID int64, splitID automationstep.ID) []automationstep.ID {
	c.t.Helper()
	runs := byOrdinalOf(c.t, c.env, automationID, fixtures.ContactAliceID)
	return lo.Map(runs, func(r *ent.AutomationRun, _ int) automationstep.ID {
		return tookPath(recordOf(c.t, c.env, r.ID, splitID))
	})
}

// yearlyBirthday is a yearly date Trigger on the birthday, at 10:00.
func yearlyBirthday() datetrigger.Date {
	return datetrigger.Date{Field: datetrigger.Field{Kind: datetrigger.Birthday}, Recurrence: localcalendar.Yearly, LocalTime: "10:00"}
}

// A Split in a yearly date-triggered Automation draws on the anniversary year in place of the
// ordinal (ADR 0126): at the pinned Split Alice draws 65 for 2030, the holdout, and 18 for 2031, the
// variant, where her first and second Enrollments' ordinals would draw 39 and 91 (shasum). A twin
// Automation published after her 2030 birthday enrolls her first in 2031, at ordinal 1, and its draw
// for the same year lands on the same path.
func TestASplitInADateTriggeredAutomationDrawsAgainEachYear(t *testing.T) {
	c := newDatedSplit(t, time.Date(2030, time.March, 1, 12, 0, 0, 0, time.UTC))
	auto := c.publish(yearlyBirthday(), fiftyFifty())
	c.tickAt(time.Date(2030, time.March, 14, 10, 1, 0, 0, time.UTC))
	c.clock.Set(time.Date(2030, time.June, 1, 12, 0, 0, 0, time.UTC))
	twin := c.publish(yearlyBirthday(), fiftyFifty())

	c.tickAt(time.Date(2031, time.March, 14, 10, 1, 0, 0, time.UTC))

	paths := c.pathsOf(auto, pinnedSplitID)
	twinPaths := c.pathsOf(twin, pinnedSplitID)
	assert.Equal(t, []automationstep.ID{holdoutPathID, variantPathID}, paths, "2030 holds her out, 2031 draws her again")
	assert.Equal(t, []automationstep.ID{variantPathID}, twinPaths, "the same Contact and year draw the same path")
}

// onceSplitID is a Split's id at which Alice's draw for the ordinal 1 is 6, the variant, and for the
// once mark 0 would be 98, the holdout (shasum).
const onceSplitID = automationstep.ID("5c1d0e2f-8a4b-4c3d-9e2f-000000000010")

// A Split in a once date-triggered Automation draws on the Enrollment's ordinal (ADR 0126): Alice's
// one Enrollment, on her `trial_ends_at`, takes the ordinal 1 and draws the variant.
func TestASplitInAOnceDateTriggeredAutomationDrawsOnTheOrdinal(t *testing.T) {
	c := newDatedSplit(t, time.Date(2030, time.March, 1, 12, 0, 0, 0, time.UTC))
	alice, err := c.env.DB.Contact.Get(t.Context(), fixtures.ContactAliceID)
	require.NoError(t, err)
	_, err = testhelper.Module[*contacts.Module](c.env).Update(t.Context(), c.env.DB.Scoped(fixtures.AcmeID), alice.ID, contacts.UpdateInput{
		Revision: alice.Revision, CustomFields: patch.Set(map[string]any{fixtures.CustomFieldAcmeTrialEndsAtKey: "2030-03-14T00:00:00Z"}),
	})
	require.NoError(t, err)
	split := fiftyFifty()
	split.ID = onceSplitID
	trialEnds := datetrigger.Date{
		Field:      datetrigger.Field{Kind: datetrigger.CustomField, Key: fixtures.CustomFieldAcmeTrialEndsAtKey},
		Recurrence: localcalendar.Once, LocalTime: "10:00",
	}
	auto := c.publish(trialEnds, split)

	c.tickAt(time.Date(2030, time.March, 14, 10, 1, 0, 0, time.UTC))

	paths := c.pathsOf(auto, onceSplitID)
	assert.Equal(t, []automationstep.ID{variantPathID}, paths)
}
