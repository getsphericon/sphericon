package accounts_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"pgregory.net/rapid"

	"github.com/getsphericon/sphericon/internal/accounts"
)

// everythingAllows is the one set of facts that joins; each branch test breaks one fact.
func everythingAllows() accounts.JoinFacts {
	return accounts.JoinFacts{ProvenNow: true, DomainHeld: true, SwitchOn: true, SignupPermits: true}
}

func TestDecideJoinAnAddressProvenNowOnAHeldDomainWithTheSwitchOnJoins(t *testing.T) {
	got := accounts.DecideJoin(everythingAllows())

	assert.Equal(t, accounts.JoinDecision{Outcome: accounts.JoinAsMember}, got)
}

func TestDecideJoinAnAddressNotProvenNowNeverJoins(t *testing.T) {
	facts := everythingAllows()
	facts.ProvenNow = false

	got := accounts.DecideJoin(facts)

	assert.Equal(t, accounts.JoinDecision{Reason: accounts.JoinReasonNotProvenNow}, got)
}

func TestDecideJoinADomainNoWorkspaceHoldsVerifiedJoinsNothing(t *testing.T) {
	facts := everythingAllows()
	facts.DomainHeld = false

	got := accounts.DecideJoin(facts)

	assert.Equal(t, accounts.JoinDecision{Reason: accounts.JoinReasonNoVerifiedHold}, got)
}

func TestDecideJoinASwitchedOffWorkspaceAdmitsNobody(t *testing.T) {
	facts := everythingAllows()
	facts.SwitchOn = false

	got := accounts.DecideJoin(facts)

	assert.Equal(t, accounts.JoinDecision{Reason: accounts.JoinReasonSwitchOff}, got)
}

func TestDecideJoinASuspendedWorkspaceAdmitsNobody(t *testing.T) {
	facts := everythingAllows()
	facts.Suspended = true

	got := accounts.DecideJoin(facts)

	assert.Equal(t, accounts.JoinDecision{Reason: accounts.JoinReasonWorkspaceFrozen}, got)
}

func TestDecideJoinAWorkspaceInItsDeletionWindowAdmitsNobody(t *testing.T) {
	facts := everythingAllows()
	facts.InDeletion = true

	got := accounts.DecideJoin(facts)

	assert.Equal(t, accounts.JoinDecision{Reason: accounts.JoinReasonWorkspaceFrozen}, got)
}

func TestDecideJoinAMemberOfAnyRoleIsNotJoinedAgain(t *testing.T) {
	facts := everythingAllows()
	facts.HasMembership = true

	got := accounts.DecideJoin(facts)

	assert.Equal(t, accounts.JoinDecision{Reason: accounts.JoinReasonAlreadyMember}, got)
}

func TestDecideJoinAClosedSignupStaysClosedWhateverDomainMatches(t *testing.T) {
	facts := everythingAllows()
	facts.SignupPermits = false

	got := accounts.DecideJoin(facts)

	assert.Equal(t, accounts.JoinDecision{Reason: accounts.JoinReasonSignupClosed}, got)
}

func TestDecideJoinJoinsExactlyWhenEveryFactAllows(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		facts := drawJoinFacts(t)

		got := accounts.DecideJoin(facts)

		allows := facts.ProvenNow && facts.DomainHeld && facts.SwitchOn && !facts.Suspended &&
			!facts.InDeletion && !facts.HasMembership && facts.SignupPermits
		require.Equal(t, allows, got.Outcome == accounts.JoinAsMember)
	})
}

func TestDecideJoinNamesAReasonExactlyWhenItJoinsNothing(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		got := accounts.DecideJoin(drawJoinFacts(t))

		require.Equal(t, got.Outcome == accounts.JoinNothing, got.Reason != "")
	})
}

func TestDecideJoinAnAddressNotProvenNowNeverJoinsWhateverElseIsTrue(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		facts := drawJoinFacts(t)
		facts.ProvenNow = false

		got := accounts.DecideJoin(facts)

		require.Equal(t, accounts.JoinNothing, got.Outcome)
	})
}

func drawJoinFacts(t *rapid.T) accounts.JoinFacts {
	return accounts.JoinFacts{
		ProvenNow:     rapid.Bool().Draw(t, "provenNow"),
		DomainHeld:    rapid.Bool().Draw(t, "domainHeld"),
		SwitchOn:      rapid.Bool().Draw(t, "switchOn"),
		Suspended:     rapid.Bool().Draw(t, "suspended"),
		InDeletion:    rapid.Bool().Draw(t, "inDeletion"),
		HasMembership: rapid.Bool().Draw(t, "hasMembership"),
		SignupPermits: rapid.Bool().Draw(t, "signupPermits"),
	}
}
