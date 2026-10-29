package accounts

import "github.com/getsphericon/sphericon/internal/signin"

// SigninFacts is everything the sign-in decision of a provider callback rests on, read
// before it is made (ADR 0048). The decision is a pure function of them, so every branch is
// tested on its inputs alone, and what each branch does to the database is SignInWithIdentity's.
type SigninFacts struct {
	// KnownIdentity: a Linked identity with the provider's subject exists.
	KnownIdentity bool
	// TrustFailure is why the provider's proof of the email is not accepted ("" when it is).
	TrustFailure signin.Reason
	// EmailHolder is the User that already holds the provider's email, nil when none does.
	EmailHolder *EmailHolder
	// SignupOpen is whether the Instance lets new Users sign up (ADR 0042).
	SignupOpen bool
}

// EmailHolder is what the decision needs to know of the User who holds the address.
type EmailHolder struct {
	// Verified: the User's email is Verified (email_verified_at is set).
	Verified bool
	// Proven: the address was proven by a link or a provider (email_proven_at is set), not
	// merely recorded as verified because the install turned verification off.
	Proven bool
}

// SigninOutcome is what a provider callback does with the person.
type SigninOutcome int

const (
	// SigninRefuse: no session; the Reason says why.
	SigninRefuse SigninOutcome = iota
	// SigninKnown: the subject is a Linked identity of a User; sign that User in.
	SigninKnown
	// SigninLink: a trusted email matches a Verified User; link the identity to it.
	SigninLink
	// SigninTakeOver: a trusted email matches an Unverified User; the provider's proof wins.
	SigninTakeOver
	// SigninCreate: a trusted email nobody holds; create a Verified User with the identity.
	SigninCreate
)

// SigninDecision is an outcome with the Reason of a refusal.
type SigninDecision struct {
	Outcome SigninOutcome
	Reason  signin.Reason
}

// DecideSignin is the one sign-in decision of the provider callback. A Linked identity
// signs its User in whatever the provider says of the email now (a person who changed
// the address at the provider keeps the account); a provider whose email proof is not
// accepted creates and links nothing; a trusted email held by a User links to it (Verified
// holder) or takes it over (Unverified holder); an address nobody holds creates a User if
// sign-up is open.
func DecideSignin(f SigninFacts) SigninDecision {
	switch {
	case f.KnownIdentity:
		return SigninDecision{Outcome: SigninKnown}
	case f.TrustFailure != "":
		return SigninDecision{Outcome: SigninRefuse, Reason: f.TrustFailure}
	case f.EmailHolder != nil && f.EmailHolder.Verified:
		return SigninDecision{Outcome: SigninLink}
	case f.EmailHolder != nil:
		return SigninDecision{Outcome: SigninTakeOver}
	case !f.SignupOpen:
		return SigninDecision{Outcome: SigninRefuse, Reason: signin.ReasonSignupClosed}
	default:
		return SigninDecision{Outcome: SigninCreate}
	}
}
