package accounts_test

import (
	"testing"

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
	facts := accounts.SigninFacts{SignupOpen: true, EmailHolder: &accounts.EmailHolder{Verified: true, Proven: true}}

	got := accounts.DecideSignin(facts)

	assert.Equal(t, accounts.SigninLink, got.Outcome)
}

func TestDecideSigninATrustedEmailOfAnUnprovenVerifiedUserStillLinksAndIsTheLinkersToClear(t *testing.T) {
	facts := accounts.SigninFacts{SignupOpen: true, EmailHolder: &accounts.EmailHolder{Verified: true, Proven: false}}

	got := accounts.DecideSignin(facts)

	assert.Equal(t, accounts.SigninLink, got.Outcome)
}

func TestDecideSigninATrustedEmailOfAnUnverifiedUserTakesItOver(t *testing.T) {
	facts := accounts.SigninFacts{SignupOpen: true, EmailHolder: &accounts.EmailHolder{}}

	got := accounts.DecideSignin(facts)

	assert.Equal(t, accounts.SigninTakeOver, got.Outcome)
}

func TestDecideSigninAnAddressNobodyHoldsCreatesWhileSignupIsOpen(t *testing.T) {
	got := accounts.DecideSignin(accounts.SigninFacts{SignupOpen: true})

	assert.Equal(t, accounts.SigninCreate, got.Outcome)
}

func TestDecideSigninAnAddressNobodyHoldsIsRefusedWhileSignupIsClosed(t *testing.T) {
	got := accounts.DecideSignin(accounts.SigninFacts{SignupOpen: false})

	assert.Equal(t, accounts.SigninDecision{Outcome: accounts.SigninRefuse, Reason: signin.ReasonSignupClosed}, got)
}

// Closed sign-up stops a new User only: it never keeps an existing account out.
func TestDecideSigninClosedSignupDoesNotStopALinkOrATakeOver(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		facts := drawFacts(t)
		facts.KnownIdentity = false
		facts.TrustFailure = ""
		facts.SignupOpen = false
		facts.EmailHolder = &accounts.EmailHolder{Verified: rapid.Bool().Draw(t, "verified"), Proven: rapid.Bool().Draw(t, "proven")}

		got := accounts.DecideSignin(facts)

		require.Contains(t, []accounts.SigninOutcome{accounts.SigninLink, accounts.SigninTakeOver}, got.Outcome)
	})
}

func drawFacts(t *rapid.T) accounts.SigninFacts {
	facts := accounts.SigninFacts{
		KnownIdentity: rapid.Bool().Draw(t, "known"),
		TrustFailure:  rapid.SampledFrom([]signin.Reason{"", signin.ReasonEmailUnverified, signin.ReasonEmailNotTrusted}).Draw(t, "trust"),
		SignupOpen:    rapid.Bool().Draw(t, "signup"),
	}
	facts.EmailHolder = rapid.SampledFrom([]*accounts.EmailHolder{nil, {}, {Verified: true}, {Verified: true, Proven: true}}).Draw(t, "holder")
	return facts
}
