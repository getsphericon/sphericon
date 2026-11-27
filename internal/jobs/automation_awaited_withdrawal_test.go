package jobs_test

import (
	"testing"
	"time"

	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/internal/domain/orchestration/automations"
	"github.com/getsphericon/sphericon/internal/domain/sending/outboundmessages/engagement"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/infra/events"
	"github.com/getsphericon/sphericon/internal/infra/messaging"
	"github.com/getsphericon/sphericon/internal/jobs"
	"github.com/getsphericon/sphericon/internal/kernel/automationstep"
	"github.com/getsphericon/sphericon/internal/kernel/channels"
	"github.com/getsphericon/sphericon/internal/kernel/eventmatcher"
	"github.com/getsphericon/sphericon/internal/kernel/systemevent"
	"github.com/getsphericon/sphericon/internal/testhelper"
	"github.com/getsphericon/sphericon/internal/usecase/trackingwithdrawal"
)

// A refusal of tracking reaches a kept Event too (ADR 0120, ADR 0136): an open or click a Wait for
// event kept as `awaited.<name>` is deleted with the Destination's other opens and clicks.

// The Triggers the Automations of these tests start on.
const (
	clickFlowTrigger    = "welcome.started"
	purchaseFlowTrigger = "trial.started"
)

// awaitedOf is a live Automation of Acme, started by trigger, whose Steps are steps, and the id of
// its Wait for event and of that Step's `arrived` path.
type awaitedOf struct {
	automationID  int64
	wait, arrived automationstep.ID
}

// publishedAwaiting makes an Automation of Acme started by trigger: the Steps before, a Wait for
// event of an hour under name for an Event of action, then an hour's wait and the Steps after, and
// publishes it.
func publishedAwaiting(t *testing.T, env *testhelper.TestEnv, trigger, name, action string, before, after []automationstep.Step) awaitedOf {
	t.Helper()
	ctx := t.Context()
	s := env.DB.Scoped(fixtures.AcmeID)
	m := testhelper.Module[*automations.Module](env)
	wait := automationstep.Step{ID: automationstep.NewID(), Payload: automationstep.WaitForEvent{
		Name: name, TimeoutSeconds: 3600, Awaits: automationstep.AwaitedEvent{Actions: []string{action}},
		Arrived: plainPath(automationstep.NewID()), TimedOut: plainPath(automationstep.NewID()),
	}}
	steps := lo.Flatten([][]automationstep.Step{before, {wait, {ID: automationstep.NewID(), Payload: automationstep.Wait{Seconds: 3600}}}, after})
	root := automationstep.Root(steps...)
	a, err := m.Create(ctx, s, automations.CreateInput{Name: "Awaiting " + name, Trigger: automations.EventTrigger{Matcher: eventmatcher.Matcher{Action: trigger}}, Steps: &root})
	require.NoError(t, err)
	_, err = m.Publish(ctx, s, a.ID, testhelper.AutomationRevisionIn(ctx, t, s, a.ID))
	require.NoError(t, err)
	return awaitedOf{automationID: a.ID, wait: wait.ID, arrived: wait.Payload.(automationstep.WaitForEvent).Arrived.ID}
}

// enrollOn enrolls the Contact of Acme on the Trigger's Event, occurred now on the clock, and
// returns its one Enrollment in the Automation.
func enrollOn(t *testing.T, env *testhelper.TestEnv, clock *harnessClock, trigger string, automationID, contactID int64) int64 {
	t.Helper()
	err := testhelper.Module[*automations.StepExecution](env).OnEvent(t.Context(), env.DB.Scoped(fixtures.AcmeID),
		automations.OnEventInput{ContactID: contactID, Event: automations.Event{Action: trigger, OccurredAt: clock.now}})
	require.NoError(t, err)
	runIDs := enrollmentsOf(t, env, automationID, contactID)
	require.Len(t, runIDs, 1)
	return runIDs[0]
}

// clickAs records a person's click on a link of the message, and works the trigger job the click
// queues, which wakes a Wait for event that awaits it.
func clickAs(t *testing.T, env *testhelper.TestEnv, messageID int64) {
	t.Helper()
	link, err := env.Tracker.ClickURL(env.Tracked(t, messageID), "https://example.com/offer")
	require.NoError(t, err)
	hit := engagement.Hit{UserAgent: browserUA, Address: "192.0.2.10", MayRecord: func() bool { return true }}
	_, err = testhelper.Module[*engagement.Recorder](env).Click(t.Context(), link[len(env.Tracker.Origin()+"/e/c/"):], hit)
	require.NoError(t, err)
	env.PerformEnqueuedJobs(t, jobs.EvaluateTriggerArgs{})
}

// orderAs records the Contact's order, occurred now on the clock, and works the trigger job it
// queues.
func orderAs(t *testing.T, env *testhelper.TestEnv, clock *harnessClock, contactID int64) {
	t.Helper()
	ev := &events.CollectedEvent{WorkspaceID: fixtures.AcmeID, ContactID: contactID, Action: "order.created", Properties: map[string]any{"product": "Lamp"}, OccurredAt: clock.now}
	require.NoError(t, env.Publisher().Publish(t.Context(), ev))
	env.PerformEnqueuedJobs(t, jobs.EvaluateTriggerArgs{})
}

// keptBy is what the Enrollment's record of the Wait for event says: the action of the Event it
// keeps a copy of ("" for none) and the path it took.
func keptBy(t *testing.T, env *testhelper.TestEnv, runID int64, wait automationstep.ID) (string, automationstep.ID) {
	t.Helper()
	rec := recordOf(t, env, runID, wait)
	return lo.FromPtr(rec.AwaitedAction), tookPath(rec)
}

// thanksTo is the text of the thanks message sent to the address.
func thanksTo(t *testing.T, fs *fakeSender, address string) string {
	t.Helper()
	sent := lo.Filter(fs.sent, func(m messaging.EmailMessage, _ int) bool { return m.To == address && m.Subject == "Thanks" })
	require.Len(t, sent, 1)
	return sent[0].Text
}

// Alice and Bob each click their welcome email, and a Wait for event keeps each click; Alice also
// orders, and another Wait for event keeps the order. Alice's refusal of tracking deletes the copy
// of her click and nothing else: her path stays `arrived`, the thanks after the wait prints nothing
// for it, her order's copy and Bob's click's copy stay. A second run of the reach-back changes
// nothing more.
func TestARefusalOfTrackingDeletesTheKeptCopyOfTheDestinationsClick(t *testing.T) {
	env, clock := waitUntilEnv(t, time.Now().UTC().Truncate(time.Second))
	ctx := t.Context()
	s := env.DB.Scoped(fixtures.AcmeID)
	fs := &fakeSender{}
	stepper := stepperAt(env, newMod(env, resolvingTo(fs), clock.read), clock.read)
	welcome := welcomeEmail()
	thanks := automationstep.Step{ID: automationstep.NewID(), Payload: automationstep.Email{Subject: "Thanks", Body: "<p>Clicked [{{ awaited.click.action }}]</p>"}}
	clicks := publishedAwaiting(t, env, clickFlowTrigger, "click", systemevent.EmailClicked, []automationstep.Step{welcome}, []automationstep.Step{thanks})
	purchases := publishedAwaiting(t, env, purchaseFlowTrigger, "purchase", "order.created", nil, nil)
	alice := enrollOn(t, env, clock, clickFlowTrigger, clicks.automationID, fixtures.ContactAliceID)
	bob := enrollOn(t, env, clock, clickFlowTrigger, clicks.automationID, fixtures.ContactBobID)
	require.True(t, env.RunNextStep(t, stepper), "Alice's welcome is sent")
	require.True(t, env.RunNextStep(t, stepper), "Bob's welcome is sent")
	alicesOrder := enrollOn(t, env, clock, purchaseFlowTrigger, purchases.automationID, fixtures.ContactAliceID)

	clickAs(t, env, sentMessageOf(t, env, alice, welcome.ID))
	clickAs(t, env, sentMessageOf(t, env, bob, welcome.ID))
	orderAs(t, env, clock, fixtures.ContactAliceID)
	keptClick, path := keptBy(t, env, alice, clicks.wait)
	require.Equal(t, systemevent.EmailClicked, keptClick, "the click is kept")
	require.Equal(t, clicks.arrived, path)

	refuseTracking(t, env)

	aliceClick, alicePath := keptBy(t, env, alice, clicks.wait)
	bobClick, _ := keptBy(t, env, bob, clicks.wait)
	aliceOrder, _ := keptBy(t, env, alicesOrder, purchases.wait)
	assert.Empty(t, aliceClick, "the copy of Alice's click is deleted")
	assert.Equal(t, clicks.arrived, alicePath, "the path taken stays")
	assert.Equal(t, systemevent.EmailClicked, bobClick, "Bob's click is another Destination's")
	assert.Equal(t, "order.created", aliceOrder, "an order is no open or click")

	err := testhelper.Module[*trackingwithdrawal.UseCase](env).Withdraw(ctx, s, trackingwithdrawal.Refusal{Channel: channels.Email, Destination: fixtures.ContactAliceEmail})
	require.NoError(t, err)

	againAlice, againPath := keptBy(t, env, alice, clicks.wait)
	againBob, _ := keptBy(t, env, bob, clicks.wait)
	againOrder, _ := keptBy(t, env, alicesOrder, purchases.wait)
	assert.Equal(t, []any{"", clicks.arrived, systemevent.EmailClicked, "order.created"}, []any{againAlice, againPath, againBob, againOrder}, "a second run changes nothing more")

	clock.now = clock.now.Add(2 * time.Hour)
	env.RunQueuedSteps(t, stepper)

	assert.Contains(t, thanksTo(t, fs, fixtures.ContactAliceEmail), "Clicked []", "a later Step reads the name as absent")
	assert.Contains(t, thanksTo(t, fs, fixtures.ContactBobEmail), "Clicked [email.clicked]")
}
