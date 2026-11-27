package automations_test

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/internal/domain/orchestration/automations"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/kernel/automationstep"
	"github.com/getsphericon/sphericon/internal/kernel/conditiontree"
	"github.com/getsphericon/sphericon/internal/kernel/eventmatcher"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// The Steps of the versions these tests save: the Branch that tests a message, and the Steps it
// may name.
const (
	testerID  automationstep.ID = "5a0f6d2e-8b1c-4e3a-9f7d-000000000001"
	emailID   automationstep.ID = "5a0f6d2e-8b1c-4e3a-9f7d-000000000002"
	smsID     automationstep.ID = "5a0f6d2e-8b1c-4e3a-9f7d-000000000003"
	tagID     automationstep.ID = "5a0f6d2e-8b1c-4e3a-9f7d-000000000004"
	forkOneID automationstep.ID = "5a0f6d2e-8b1c-4e3a-9f7d-000000000005"
	siblingID automationstep.ID = "5a0f6d2e-8b1c-4e3a-9f7d-000000000006"
	ownID     automationstep.ID = "5a0f6d2e-8b1c-4e3a-9f7d-000000000007"
	laterID   automationstep.ID = "5a0f6d2e-8b1c-4e3a-9f7d-000000000008"
	goneID    automationstep.ID = "5a0f6d2e-8b1c-4e3a-9f7d-000000000009"
)

func emailStep(id automationstep.ID) automationstep.Step {
	return automationstep.Step{ID: id, Payload: automationstep.Email{Subject: "Hi", Body: "<p>Hi</p>"}}
}

func smsStep(id automationstep.ID) automationstep.Step {
	return automationstep.Step{ID: id, Payload: automationstep.SMS{Text: "Hi"}}
}

// messageTest is a Branch path's Condition that tests the message of the send Step.
func messageTest(t *testing.T, step automationstep.ID, engagement conditiontree.Engagement) conditiontree.Branch {
	t.Helper()
	raw, err := json.Marshal(map[string]any{"kind": "stepMessage", "step": step, "engagement": engagement})
	require.NoError(t, err)
	return conditiontree.Branch{Kind: conditiontree.KindGroup, Combinator: conditiontree.And, Entries: []json.RawMessage{raw}}
}

// testingBranch is the Branch testerID, its one listed path on the Condition and holding the Steps
// given, with an unknown path.
func testingBranch(cond conditiontree.Branch, own ...automationstep.Step) automationstep.Step {
	return automationstep.Step{ID: testerID, Payload: automationstep.Branch{
		Paths:     []automationstep.BranchPath{{Condition: cond, Steps: append([]automationstep.Step{}, own...)}},
		Otherwise: automationstep.Path{Steps: []automationstep.Step{}},
		Unknown:   &automationstep.Path{Steps: []automationstep.Step{}},
	}}
}

// version is the root path of a version: an email Step, an SMS Step and a tag Step, then a Branch
// whose paths hold the Steps siblingID (its first path) and nothing, then the Steps given.
func version(t *testing.T, rest ...automationstep.Step) automationstep.Path {
	t.Helper()
	earlier := automationstep.Step{ID: forkOneID, Payload: automationstep.Branch{
		Paths:     []automationstep.BranchPath{{Condition: messageTest(t, emailID, conditiontree.MessageClicked), Steps: []automationstep.Step{emailStep(siblingID)}}},
		Otherwise: automationstep.Path{Steps: []automationstep.Step{}},
		Unknown:   &automationstep.Path{Steps: []automationstep.Step{}},
	}}
	steps := append([]automationstep.Step{emailStep(emailID), smsStep(smsID), {ID: tagID, Payload: automationstep.ApplyTag{Tag: "vip"}}, earlier}, rest...)
	return automationstep.Root(steps...)
}

// saveVersion creates a draft Automation of Acme with the Steps.
func saveVersion(t *testing.T, env *testhelper.TestEnv, steps automationstep.Path) error {
	t.Helper()
	_, err := testhelper.Module[*automations.Module](env).Create(t.Context(), env.DB.Scoped(fixtures.AcmeID),
		automations.CreateInput{Name: "Message tests", Trigger: automations.EventTrigger{Matcher: eventmatcher.Matcher{Action: "signed_up"}}, Steps: &steps})
	return err
}

// A Branch may test the message of a send Step that can come before it: an email Step's open or
// click, any send Step's click, and one in a path of an earlier Branch that rejoined before it.
func TestABranchTestsTheMessageOfASendStepBeforeIt(t *testing.T) {
	env := testhelper.Setup(t)
	for name, cond := range map[string]conditiontree.Branch{
		"an email's open":             messageTest(t, emailID, conditiontree.MessageOpened),
		"an email's click":            messageTest(t, emailID, conditiontree.MessageClicked),
		"a text's click":              messageTest(t, smsID, conditiontree.MessageClicked),
		"an email of an earlier fork": messageTest(t, siblingID, conditiontree.MessageOpened),
	} {
		t.Run(name, func(t *testing.T) {
			err := saveVersion(t, env, version(t, testingBranch(cond)))

			require.NoError(t, err)
		})
	}
}

// A Branch is refused, naming it and its path, when its test of a message names a Step that is not
// a send Step that can come before it: one after it, one of its own paths, one the version does not
// hold (a send Step removed while a test names it), or a Step that sends nothing; and when it asks
// whether a message that is no email was opened.
func TestABranchsMessageTestNamesASendStepThatCanComeBeforeIt(t *testing.T) {
	env := testhelper.Setup(t)
	refusal := "step " + string(testerID) + " (branch): path 1: "
	for name, c := range map[string]struct {
		steps automationstep.Path
		names string
	}{
		"a Step after it": {version(t, testingBranch(messageTest(t, laterID, conditiontree.MessageOpened)), emailStep(laterID)),
			refusal + "the message test names step " + string(laterID) + ", which is no send step before the branch"},
		"a Step of its own path": {version(t, testingBranch(messageTest(t, ownID, conditiontree.MessageOpened), emailStep(ownID))),
			refusal + "the message test names step " + string(ownID) + ", which is no send step before the branch"},
		"a Step the version does not hold": {version(t, testingBranch(messageTest(t, goneID, conditiontree.MessageClicked))),
			refusal + "the message test names step " + string(goneID) + ", which is no send step before the branch"},
		"a Step that sends nothing": {version(t, testingBranch(messageTest(t, tagID, conditiontree.MessageClicked))),
			refusal + "the message test names step " + string(tagID) + " (apply_tag), which sends no message"},
		"the open of a text": {version(t, testingBranch(messageTest(t, smsID, conditiontree.MessageOpened))),
			refusal + "only an email step's message is opened, step " + string(smsID) + " sends sms"},
	} {
		t.Run(name, func(t *testing.T) {
			err := saveVersion(t, env, c.steps)

			require.ErrorIs(t, err, automations.ErrInvalidStep)
			require.ErrorContains(t, err, c.names)
		})
	}
}

// A test of the Enrollment's own message can be unknown, so a Branch that holds one is published only
// with its unknown path (ADR 0122); a draft keeps it without one.
func TestABranchTestingAMessageIsPublishedOnlyWithItsUnknownPath(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := t.Context()
	s := env.DB.Scoped(fixtures.AcmeID)
	m := testhelper.Module[*automations.Module](env)
	branch := testingBranch(messageTest(t, emailID, conditiontree.MessageOpened))
	b := branch.Payload.(automationstep.Branch)
	b.Unknown = nil
	branch.Payload = b
	steps := automationstep.Root(emailStep(emailID), branch)
	a, err := m.Create(ctx, s, automations.CreateInput{Name: "No unknown path", Trigger: automations.EventTrigger{Matcher: eventmatcher.Matcher{Action: "signed_up"}}, Steps: &steps})
	require.NoError(t, err, "a draft keeps a Branch without its unknown path")

	_, err = m.Publish(ctx, s, a.ID, testhelper.AutomationRevisionIn(ctx, t, s, a.ID))

	require.ErrorIs(t, err, automations.ErrInvalidStep)
	require.ErrorContains(t, err, "step "+string(testerID)+" (branch): a branch whose conditions count an open or a click, or test a message of the enrollment, needs an unknown path")
}
