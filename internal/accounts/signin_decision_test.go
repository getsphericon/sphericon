package accounts_test

import (
	"testing"

	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"pgregory.net/rapid"

	"github.com/getsphericon/sphericon/internal/accounts"
	"github.com/getsphericon/sphericon/internal/signin"
)

func TestDecideSigninAKnownIdentitySignsInWhateverElseIsTrue(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		facts := drawFacts(t)
		facts.KnownIdentity = true

		got := accounts.DecideSignin(facts)

		require.Equal(t, accounts.SigninKnown, got.Outcome)
	})
}

func TestDecideSigninAnUntrustedEmailCreatesAndLinksNothing(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		facts := drawFacts(t)
		facts.KnownIdentity = false
		facts.TrustFailure = rapid.SampledFrom([]signin.Reason{signin.ReasonEmailUnverified, signin.ReasonEmailNotTrusted}).Draw(t, "failure")

		got := accounts.DecideSignin(facts)

		require.Equal(t, accounts.SigninDecision{Outcome: accounts.SigninRefuse, Reason: facts.TrustFailure}, got)
	})
}

func TestDecideSigninATrustedEmailOfAVerifiedUserLinks(t *testing.T) {
	facts := accounts.SigninFacts{EmailHolder: &accounts.EmailHolder{Verified: true, Proven: true}}

	got := accounts.DecideSignin(facts)

	assert.Equal(t, accounts.SigninLink, got.Outcome)
}

func TestDecideSigninATrustedEmailOfAnUnprovenVerifiedUserStillLinksAndIsTheLinkersToClear(t *testing.T) {
	facts := accounts.SigninFacts{EmailHolder: &accounts.EmailHolder{Verified: true, Proven: false}}

	got := accounts.DecideSignin(facts)

	assert.Equal(t, accounts.SigninLink, got.Outcome)
}

func TestDecideSigninATrustedEmailOfAnUnverifiedUserTakesItOver(t *testing.T) {
	facts := accounts.SigninFacts{EmailHolder: &accounts.EmailHolder{}}

	got := accounts.DecideSignin(facts)

	assert.Equal(t, accounts.SigninTakeOver, got.Outcome)
}

// ADR 0063: sign-up is always open, so an address nobody holds creates a User.
func TestDecideSigninAnAddressNobodyHoldsCreates(t *testing.T) {
	got := accounts.DecideSignin(accounts.SigninFacts{})

	assert.Equal(t, accounts.SigninDecision{Outcome: accounts.SigninCreate}, got)
}

// Only a provider whose proof of the email is not accepted is refused, and only for a subject
// that is no Linked identity.
func TestDecideSigninRefusesExactlyAnUntrustedEmailOfAnUnknownSubject(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		facts := drawFacts(t)

		got := accounts.DecideSignin(facts)

		refused := !facts.KnownIdentity && facts.TrustFailure != ""
		require.Equal(t, refused, got.Outcome == accounts.SigninRefuse)
	})
}

func TestDecideSigninATrustedEmailOfAnUnknownSubjectCreatesOnlyWhenNobodyHoldsItAndNoItemsAreAsked(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		facts := drawFacts(t)
		facts.KnownIdentity = false
		facts.TrustFailure = ""

		got := accounts.DecideSignin(facts)

		require.Equal(t, facts.EmailHolder == nil && !facts.SignupItemsInEffect, got.Outcome == accounts.SigninCreate)
	})
}

// ADR 0062: a Sign-in provider creates no User before the Sign-up items are answered.
func TestDecideSigninAnAddressNobodyHoldsIsHeldPendingWhileSignupItemsAreInEffect(t *testing.T) {
	got := accounts.DecideSignin(accounts.SigninFacts{SignupItemsInEffect: true})

	assert.Equal(t, accounts.SigninDecision{Outcome: accounts.SigninPending}, got)
}

// ADR 0062: taking over an Unverified User makes it a Verified User of the Instance, so the person
// answers the Sign-up items first, as a new User does.
func TestDecideSigninATakeoverIsHeldPendingWhileSignupItemsAreInEffect(t *testing.T) {
	got := accounts.DecideSignin(accounts.SigninFacts{EmailHolder: &accounts.EmailHolder{}, SignupItemsInEffect: true})

	assert.Equal(t, accounts.SigninDecision{Outcome: accounts.SigninPending}, got)
}

// A sign-up is held pending exactly where it would create a User or take an Unverified one over:
// the items never stop a Linked identity or a link, and never turn a refusal into a pending
// sign-up.
func TestDecideSigninHoldsPendingExactlyWhatWouldBeCreatedOrTakenOverWhileItemsAreInEffect(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		facts := drawFacts(t)
		without := facts
		without.SignupItemsInEffect = false

		got := accounts.DecideSignin(facts)

		makesAUser := lo.Contains([]accounts.SigninOutcome{accounts.SigninCreate, accounts.SigninTakeOver}, accounts.DecideSignin(without).Outcome)
		require.Equal(t, makesAUser && facts.SignupItemsInEffect, got.Outcome == accounts.SigninPending)
	})
}

// While the Sign-up items are in effect no provider sign-in makes a Verified User of an address on
// its own: it neither creates one nor takes one over.
func TestDecideSigninNeitherCreatesNorTakesOverWhileItemsAreInEffect(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		facts := drawFacts(t)
		facts.SignupItemsInEffect = true

		got := accounts.DecideSignin(facts)

		require.NotContains(t, []accounts.SigninOutcome{accounts.SigninCreate, accounts.SigninTakeOver}, got.Outcome)
	})
}

func drawFacts(t *rapid.T) accounts.SigninFacts {
	facts := accounts.SigninFacts{
		KnownIdentity: rapid.Bool().Draw(t, "known"),
		TrustFailure:  rapid.SampledFrom([]signin.Reason{"", signin.ReasonEmailUnverified, signin.ReasonEmailNotTrusted}).Draw(t, "trust"),

		SignupItemsInEffect: rapid.Bool().Draw(t, "items"),
	}
	facts.EmailHolder = rapid.SampledFrom([]*accounts.EmailHolder{nil, {}, {Verified: true}, {Verified: true, Proven: true}}).Draw(t, "holder")
	return facts
}
