package users_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"pgregory.net/rapid"

	"github.com/getsphericon/sphericon/internal/users"
)

// everythingAllows is the one set of facts that joins; each branch test breaks one fact.
func everythingAllows() users.JoinFacts {
	return users.JoinFacts{ProvenNow: true, DomainHeld: true, SwitchOn: true}
}

func TestDecideJoinAnAddressProvenNowOnAHeldDomainWithTheSwitchOnJoins(t *testing.T) {
	got := users.DecideJoin(everythingAllows())

	assert.Equal(t, users.JoinDecision{Outcome: users.JoinAsMember}, got)
}

func TestDecideJoinAnAddressNotProvenNowNeverJoins(t *testing.T) {
	facts := everythingAllows()
	facts.ProvenNow = false

	got := users.DecideJoin(facts)

	assert.Equal(t, users.JoinDecision{Reason: users.JoinReasonNotProvenNow}, got)
}

func TestDecideJoinADomainNoWorkspaceHoldsVerifiedJoinsNothing(t *testing.T) {
	facts := everythingAllows()
	facts.DomainHeld = false

	got := users.DecideJoin(facts)

	assert.Equal(t, users.JoinDecision{Reason: users.JoinReasonNoVerifiedHold}, got)
}

func TestDecideJoinASwitchedOffWorkspaceAdmitsNobody(t *testing.T) {
	facts := everythingAllows()
	facts.SwitchOn = false

	got := users.DecideJoin(facts)

	assert.Equal(t, users.JoinDecision{Reason: users.JoinReasonSwitchOff}, got)
}

func TestDecideJoinASuspendedWorkspaceAdmitsNobody(t *testing.T) {
	facts := everythingAllows()
	facts.Suspended = true

	got := users.DecideJoin(facts)

	assert.Equal(t, users.JoinDecision{Reason: users.JoinReasonWorkspaceFrozen}, got)
}

func TestDecideJoinAWorkspaceInItsDeletionWindowAdmitsNobody(t *testing.T) {
	facts := everythingAllows()
	facts.InDeletion = true

	got := users.DecideJoin(facts)

	assert.Equal(t, users.JoinDecision{Reason: users.JoinReasonWorkspaceFrozen}, got)
}

func TestDecideJoinAMemberOfAnyRoleIsNotJoinedAgain(t *testing.T) {
	facts := everythingAllows()
	facts.HasMembership = true

	got := users.DecideJoin(facts)

	assert.Equal(t, users.JoinDecision{Reason: users.JoinReasonAlreadyMember}, got)
}

func TestDecideJoinJoinsExactlyWhenEveryFactAllows(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		facts := drawJoinFacts(t)

		got := users.DecideJoin(facts)

		allows := facts.ProvenNow && facts.DomainHeld && facts.SwitchOn && !facts.Suspended &&
			!facts.InDeletion && !facts.HasMembership
		require.Equal(t, allows, got.Outcome == users.JoinAsMember)
	})
}

func TestDecideJoinNamesAReasonExactlyWhenItJoinsNothing(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		got := users.DecideJoin(drawJoinFacts(t))

		require.Equal(t, got.Outcome == users.JoinNothing, got.Reason != "")
	})
}

func TestDecideJoinAnAddressNotProvenNowNeverJoinsWhateverElseIsTrue(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		facts := drawJoinFacts(t)
		facts.ProvenNow = false

		got := users.DecideJoin(facts)

		require.Equal(t, users.JoinNothing, got.Outcome)
	})
}

func drawJoinFacts(t *rapid.T) users.JoinFacts {
	return users.JoinFacts{
		ProvenNow:     rapid.Bool().Draw(t, "provenNow"),
		DomainHeld:    rapid.Bool().Draw(t, "domainHeld"),
		SwitchOn:      rapid.Bool().Draw(t, "switchOn"),
		Suspended:     rapid.Bool().Draw(t, "suspended"),
		InDeletion:    rapid.Bool().Draw(t, "inDeletion"),
		HasMembership: rapid.Bool().Draw(t, "hasMembership"),
	}
}
