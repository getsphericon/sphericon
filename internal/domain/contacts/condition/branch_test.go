package condition_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/internal/domain/contacts/condition"
	"github.com/getsphericon/sphericon/internal/domain/contacts/engagementtracking"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/kernel/channels"
	"github.com/getsphericon/sphericon/internal/kernel/conditiontree"
	"github.com/getsphericon/sphericon/internal/kernel/systemevent"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// branchOf is a Branch path's Condition: the and-group of entries.
func branchOf(entries ...any) conditiontree.Branch {
	return conditiontree.Branch{Group: opsGroup(entries...)}
}

// A Branch's Conditions are each read three-valued for the Contact, in one read: an engagement
// count of an address that refused tracking is unknown, a test of the Contact holds or fails, and
// a test of the Event the Enrollment began with reads its properties. Ann refused tracking.
func TestABranchsConditionsAreReadTrueFalseOrUnknown(t *testing.T) {
	env := testhelper.Setup(t)
	record(t, env, fixtures.MassiveID, channels.Email, fixtures.MassiveAnnEmail, engagementtracking.Denied)
	conds := []conditiontree.Branch{
		branchOf(eventLeaf(systemevent.EmailOpened, "atLeast", 1, 0)),
		branchOf(attribute("first_name", text("eq", "Ann"))),
		branchOf(attribute("first_name", text("eq", "Bob"))),
		branchOf(property("amount", text("eq", "10"))),
		branchOf(negated(eventLeaf(systemevent.EmailOpened, "atLeast", 1, 0))),
	}

	got, err := condition.New(time.Now).Outcomes(t.Context(), env.DB.Scoped(fixtures.MassiveID), condition.OutcomesInput{ContactID: ann, Conditions: conds, Properties: map[string]any{"amount": "10"}})

	require.NoError(t, err)
	assert.Equal(t, []conditiontree.Truth{
		conditiontree.TruthUnknown, conditiontree.TruthTrue, conditiontree.TruthFalse, conditiontree.TruthTrue, conditiontree.TruthUnknown,
	}, got)
}

// Only engagement is unknown: a test of a field the Contact has no value of fails. Tara has no
// email address, so `email neq` reads no value for her, and is false.
func TestATestThatReadsNoValueIsFalseInABranch(t *testing.T) {
	env := testhelper.Setup(t)
	conds := []conditiontree.Branch{
		branchOf(attribute("email", text("neq", "x@example.com"))),
		branchOf(negated(attribute("email", text("neq", "x@example.com")))),
		branchOf(attribute("email", text("neq", "x@example.com")), eventLeaf(systemevent.TelegramClicked, "atMost", 0, 0)),
	}

	got, err := condition.New(time.Now).Outcomes(t.Context(), env.DB.Scoped(fixtures.DuffID), condition.OutcomesInput{ContactID: tara, Conditions: conds})

	require.NoError(t, err)
	assert.Equal(t, []conditiontree.Truth{conditiontree.TruthFalse, conditiontree.TruthFalse, conditiontree.TruthFalse}, got)
}

// A Branch path's Condition is checked as a Rule's is: a Custom field it names must be the
// Workspace's.
func TestABranchsConditionNamesOnlyTheWorkspacesCustomFields(t *testing.T) {
	env := testhelper.Setup(t)

	err := condition.New(time.Now).CheckBranch(t.Context(), env.DB.Scoped(fixtures.AcmeID), branchOf(customField("no_such_field", presence("set"))))

	require.ErrorContains(t, err, `unknown custom field "no_such_field"`)
}
