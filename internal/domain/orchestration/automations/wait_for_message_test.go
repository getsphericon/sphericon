package automations_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/internal/domain/orchestration/automations"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/kernel/automationstep"
	"github.com/getsphericon/sphericon/internal/kernel/conditiontree"
	"github.com/getsphericon/sphericon/internal/kernel/eventmatcher"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// waiterID is the Wait for event these tests save.
const waiterID automationstep.ID = "5a0f6d2e-8b1c-4e3a-9f7d-00000000000a"

// waitingFor is the Wait for event waiterID on the open or click of the send Step's message, its
// arrived path holding the Steps given, with an unknown path.
func waitingFor(step automationstep.ID, engagement conditiontree.Engagement, own ...automationstep.Step) automationstep.Step {
	return automationstep.Step{ID: waiterID, Payload: automationstep.WaitForEvent{
		Name: "welcome_open", TimeoutSeconds: 3600,
		Awaits:   automationstep.AwaitedMessage{Step: step, Engagement: engagement},
		Arrived:  automationstep.Path{Steps: append([]automationstep.Step{}, own...)},
		TimedOut: automationstep.Path{Steps: []automationstep.Step{}},
		Unknown:  &automationstep.Path{Steps: []automationstep.Step{}},
	}}
}

// withoutUnknown is the Wait for event with no unknown path.
func withoutUnknown(st automationstep.Step) automationstep.Step {
	w := st.Payload.(automationstep.WaitForEvent)
	w.Unknown = nil
	st.Payload = w
	return st
}

// A Wait for event may wait for the open or click of the message of a send Step that can come
// before it, by the rule of a Branch's test of that message: an email Step's open or click, any
// send Step's click, and one in a path of an earlier Branch that rejoined before it.
func TestAWaitForEventWaitsForTheMessageOfASendStepBeforeIt(t *testing.T) {
	env := testhelper.Setup(t)
	for name, wait := range map[string]automationstep.Step{
		"an email's open":             waitingFor(emailID, conditiontree.MessageOpened),
		"an email's click":            waitingFor(emailID, conditiontree.MessageClicked),
		"a text's click":              waitingFor(smsID, conditiontree.MessageClicked),
		"an email of an earlier fork": waitingFor(siblingID, conditiontree.MessageOpened),
	} {
		t.Run(name, func(t *testing.T) {
			err := saveVersion(t, env, version(t, wait))

			require.NoError(t, err)
		})
	}
}

// A Wait for event is refused, naming it, when the message it waits for is not one a send Step
// that can come before it sent: a Step after it, one of its own paths, one the version does not
// hold, or a Step that sends nothing; and when it waits for the open of a message that is no email.
func TestAWaitForEventsMessageIsOfASendStepThatCanComeBeforeIt(t *testing.T) {
	env := testhelper.Setup(t)
	refusal := "step " + string(waiterID) + " (wait_for_event): "
	for name, c := range map[string]struct {
		steps automationstep.Path
		names string
	}{
		"a Step after it": {version(t, waitingFor(laterID, conditiontree.MessageOpened), emailStep(laterID)),
			refusal + "it waits for the message of step " + string(laterID) + ", which is no send step before it"},
		"a Step of its own path": {version(t, waitingFor(ownID, conditiontree.MessageOpened, emailStep(ownID))),
			refusal + "it waits for the message of step " + string(ownID) + ", which is no send step before it"},
		"a Step the version does not hold": {version(t, waitingFor(goneID, conditiontree.MessageClicked)),
			refusal + "it waits for the message of step " + string(goneID) + ", which is no send step before it"},
		"a Step that sends nothing": {version(t, waitingFor(tagID, conditiontree.MessageClicked)),
			refusal + "it waits for the message of step " + string(tagID) + " (apply_tag), which sends no message"},
		"the open of a text": {version(t, waitingFor(smsID, conditiontree.MessageOpened)),
			refusal + "only an email step's message is opened, step " + string(smsID) + " sends sms"},
	} {
		t.Run(name, func(t *testing.T) {
			err := saveVersion(t, env, c.steps)

			require.ErrorIs(t, err, automations.ErrInvalidStep)
			require.ErrorContains(t, err, c.names)
		})
	}
}

// An edit that removes the send Step a Wait for event waits on, or moves the wait before it, is
// refused naming the wait.
func TestAnEditCannotTakeTheSendStepAWaitForEventWaitsOnFromBeforeIt(t *testing.T) {
	for name, edited := range map[string]automationstep.Path{
		"the send Step removed":         automationstep.Root(waitingFor(emailID, conditiontree.MessageOpened)),
		"the wait moved before the send": automationstep.Root(waitingFor(emailID, conditiontree.MessageOpened), emailStep(emailID)),
	} {
		t.Run(name, func(t *testing.T) {
			env := testhelper.Setup(t)
			ctx := t.Context()
			s := env.DB.Scoped(fixtures.AcmeID)
			m := testhelper.Module[*automations.Module](env)
			steps := automationstep.Root(emailStep(emailID), waitingFor(emailID, conditiontree.MessageOpened))
			a, err := m.Create(ctx, s, automations.CreateInput{Name: "Waits for an open", Trigger: automations.EventTrigger{Matcher: eventmatcher.Matcher{Action: "signed_up"}}, Steps: &steps})
			require.NoError(t, err)

			_, err = m.Update(ctx, s, a.ID, automations.UpdateInput{Revision: testhelper.AutomationRevisionIn(ctx, t, s, a.ID), Steps: &edited})

			require.ErrorIs(t, err, automations.ErrInvalidStep)
			require.ErrorContains(t, err, "step "+string(waiterID)+" (wait_for_event): it waits for the message of step "+string(emailID)+", which is no send step before it")
		})
	}
}

// A Wait for event on an Event is never unknown, so a save refuses it an unknown path, naming it.
func TestAWaitForAnEventTakesNoUnknownPath(t *testing.T) {
	env := testhelper.Setup(t)
	wait := waitingFor(emailID, conditiontree.MessageOpened)
	w := wait.Payload.(automationstep.WaitForEvent)
	w.Awaits = automationstep.AwaitedEvent{Actions: []string{"order.created"}}
	wait.Payload = w

	err := saveVersion(t, env, automationstep.Root(wait))

	require.ErrorIs(t, err, automations.ErrInvalidStep)
	require.ErrorContains(t, err, "step "+string(waiterID)+" (wait_for_event): a wait for an event is never unknown, so it takes no unknown path")
}

// A Wait for event on an open or click is published only with its unknown path (ADR 0124); a draft
// keeps it without one.
func TestAWaitForAMessageIsPublishedOnlyWithItsUnknownPath(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := t.Context()
	s := env.DB.Scoped(fixtures.AcmeID)
	m := testhelper.Module[*automations.Module](env)
	steps := automationstep.Root(emailStep(emailID), withoutUnknown(waitingFor(emailID, conditiontree.MessageOpened)))
	a, err := m.Create(ctx, s, automations.CreateInput{Name: "No unknown path", Trigger: automations.EventTrigger{Matcher: eventmatcher.Matcher{Action: "signed_up"}}, Steps: &steps})
	require.NoError(t, err, "a draft keeps a Wait for event without its unknown path")

	_, err = m.Publish(ctx, s, a.ID, testhelper.AutomationRevisionIn(ctx, t, s, a.ID))

	require.ErrorIs(t, err, automations.ErrInvalidStep)
	require.ErrorContains(t, err, "step "+string(waiterID)+" (wait_for_event): a wait for an open or a click needs an unknown path, taken when the app cannot know whether one happened")
}
