package condition_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/internal/domain/contacts/condition"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/kernel/automationstep"
	"github.com/getsphericon/sphericon/internal/kernel/conditiontree"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// The send Steps the tests of an Enrollment's own message name.
const (
	welcome  automationstep.ID = "0b7e3c6a-35a4-4c1e-9a52-9d1f1a7e0001"
	reminder automationstep.ID = "0b7e3c6a-35a4-4c1e-9a52-9d1f1a7e0002"
)

// stepMessage is the test of the message the send Step sent to the Enrollment: opened or clicked.
func stepMessage(step automationstep.ID, engagement string) leaf {
	return leaf{"kind": "stepMessage", "step": step, "engagement": engagement}
}

// outcomesOf reads the Conditions for Ann, the Enrollment's messages being messages.
func outcomesOf(t *testing.T, messages map[automationstep.ID]condition.SentMessage, conds ...conditiontree.Branch) []conditiontree.Truth {
	t.Helper()
	env := testhelper.Setup(t)
	got, err := condition.New(time.Now).Outcomes(t.Context(), env.DB.Scoped(fixtures.MassiveID), condition.OutcomesInput{ContactID: ann, Conditions: conds, Messages: messages})
	require.NoError(t, err)
	return got
}

// The test of an Enrollment's own message is known only while the message went out tracked and is
// tracked still: then it holds when the message was opened (or clicked) and fails when it was not.
// A message that is not tracked, or that the Enrollment was never sent, is unknown, whatever its
// stamps (ADR 0122).
func TestAStepMessageTestIsKnownOnlyForATrackedMessage(t *testing.T) {
	conds := []conditiontree.Branch{
		branchOf(stepMessage(welcome, "opened")),
		branchOf(stepMessage(welcome, "clicked")),
		branchOf(stepMessage(reminder, "opened")),
		branchOf(stepMessage(reminder, "clicked")),
		branchOf(stepMessage("0b7e3c6a-35a4-4c1e-9a52-9d1f1a7e0003", "opened")),
	}
	messages := map[automationstep.ID]condition.SentMessage{
		welcome:  {Tracked: true, Opened: true},
		reminder: {Tracked: false, Opened: true, Clicked: true},
	}

	got := outcomesOf(t, messages, conds...)

	assert.Equal(t, []conditiontree.Truth{
		conditiontree.TruthTrue, conditiontree.TruthFalse,
		conditiontree.TruthUnknown, conditiontree.TruthUnknown,
		conditiontree.TruthUnknown,
	}, got)
}

// An unknown test of the message passes through the tree by Kleene's rules: "did not open" is
// unknown too, never true; an and with a false test is false, an or with a true test is true.
func TestAnUnknownStepMessageTestComposesByKleenesRules(t *testing.T) {
	conds := []conditiontree.Branch{
		branchOf(negated(stepMessage(welcome, "opened"))),
		branchOf(stepMessage(welcome, "opened"), attribute("first_name", text("eq", "Bob"))),
		branchOf(groupOf(conditiontree.Or, false, stepMessage(welcome, "opened"), attribute("first_name", text("eq", "Ann")))),
		branchOf(stepMessage(welcome, "opened"), attribute("first_name", text("eq", "Ann"))),
	}
	messages := map[automationstep.ID]condition.SentMessage{welcome: {Tracked: false}}

	got := outcomesOf(t, messages, conds...)

	assert.Equal(t, []conditiontree.Truth{
		conditiontree.TruthUnknown, conditiontree.TruthFalse, conditiontree.TruthTrue, conditiontree.TruthUnknown,
	}, got)
}

// A Condition that tests the Enrollment's own message can be unknown, at any depth and under a
// negation too, so a Branch that holds one needs its unknown path.
func TestAStepMessageTestCanBeUnknown(t *testing.T) {
	alone := condition.CanBeUnknown(opsGroup(stepMessage(welcome, "clicked")))
	nested := condition.CanBeUnknown(opsGroup(tagLeaf("vip", "has"), negated(opsGroup(stepMessage(welcome, "opened")))))

	assert.True(t, alone)
	assert.True(t, nested)
}
