package signup

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"log/slog"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/consentrecord"
	"github.com/getsphericon/sphericon/internal/domain/identity/consentrecords"
	"github.com/getsphericon/sphericon/internal/domain/identity/users"
	"github.com/getsphericon/sphericon/internal/infra/events"
	"github.com/getsphericon/sphericon/internal/kernel/validation"
)

// SignUpInput is the Sign-up form as typed.
type SignUpInput struct {
	Name     string
	Email    string
	Password string
	// SignupItems is the answers to the Sign-up items the form showed (ADR 0062).
	SignupItems []consentrecords.Answer
}

// SignUpResult is the answer of a Sign-up that went through. With verification required, Browser
// is the Sign-up browser's value, whatever the address; without, the User is Verified with its
// first Workspace and the welcome mail is queued.
type SignUpResult struct {
	// Browser is the random value of the Sign-up browser's cookie, for a Sign-up that a
	// verification link follows; empty otherwise. Its hash is what the link carries.
	Browser string
}

// Budget is the mail budget of an address (ADR 0031): Take grants one slot and reports
// whether it was granted. A request whose slot is refused still does all the work; its mail
// job is queued but delivers nothing.
type Budget interface {
	Take(ctx context.Context, address string) (granted bool, err error)
}

// SignUpConfig is what a Sign-up needs besides the modules and the password hasher.
type SignUpConfig struct {
	// Budget is the mail budget of an address, a seam to the infrastructure's throttle.
	Budget Budget
	// RequireVerification is the email verification gate (ADR 0030): on in the saas Mode.
	RequireVerification bool
	// SurveyDue reports whether a new User owes the Sign-up survey (ADR 0055).
	SurveyDue func() bool
}

// SignUp is a person creating a User for themself with a name, an email and a password (the
// Sign-up, ADR 0030), open on every Instance (ADR 0063). It checks the form, and then, on an
// Instance that requires email verification, does the same work for every address so that
// nobody but its owner learns that an address is Taken (ADR 0031). A refused form is a
// *validation.FieldsError naming every field, the Sign-up items' answers included (ADR 0062).
//
// The order is the operation's contract:
//
//  1. the form is checked before any work, so a refused one costs no hash;
//  2. with verification required, every address costs the same: a slot of its mail budget, a
//     password hash, a lookup, a write (the create, or the replace that a Verified User's row
//     ignores) and exactly one published event, delivered only when the budget granted it. Only
//     the event differs: the request of a verification link for a new or Unverified User (the
//     link is minted when it is mailed), the claim of the address for a Verified one, whose owner
//     is told without a token;
//  3. two Sign-ups of one new address may race on the unique email: the loser finds the
//     winner's User and goes on as a repeated Sign-up of it, so a race is no answer of its own;
//  4. a repeated Sign-up of an Unverified address replaces its password, which ends every
//     earlier link, and draws a new Sign-up browser;
//  5. with verification off there is no mail to tell the owner, so a Taken address is the
//     answer (the one exception, ADR 0031), and the User is Verified at once with its first
//     Workspace.
func (m *Module) SignUp(ctx context.Context, in SignUpInput) (SignUpResult, error) {
	cfg := m.signUp
	form, formErr := users.CheckForm(in.Name, in.Email, in.Password)
	consents, err := m.consents.Check(ctx, in.SignupItems, consentrecord.SourceSignUpForm)
	if err := validation.Fields(formErr, err); err != nil {
		return SignUpResult{}, err
	}
	name, email := form.Name, form.Email
	if !cfg.RequireVerification {
		return m.signUpVerified(ctx, users.NewUser{Name: name, Email: email, SurveyDue: m.surveyDue(), Consents: consents}, in.Password)
	}

	granted, err := cfg.Budget.Take(ctx, email)
	if err != nil {
		return SignUpResult{}, err
	}
	// Hashed whatever the address is, so the work does not say whether it is Taken; a
	// Verified User's hash is dropped unused.
	hash, err := m.passwords.Hash(in.Password)
	if err != nil {
		return SignUpResult{}, err
	}
	usr, err := m.signUpUser(ctx, users.NewUser{Name: name, Email: email, PasswordHash: hash, SurveyDue: m.surveyDue(), Consents: consents})
	if err != nil {
		return SignUpResult{}, err
	}

	// The link is bound to the stored password hash, so it works until a Sign-up replaces the
	// hash or the address is verified, and it trusts the Sign-up browser drawn here. A Verified
	// User draws the same browser, and the notice to its owner carries no token.
	browser, browserHash := newBrowser()
	var request events.DomainEvent = &events.LinkRequested{
		Action: events.NameUserVerificationRequested, UserID: usr.ID, Email: usr.Email, BrowserHash: browserHash, Deliver: granted,
	}
	if usr.EmailVerifiedAt != nil {
		request = &events.AddressClaimed{Email: usr.Email, Deliver: granted}
	}
	// Best-effort: a lost request is asked for again.
	if err := m.bus.Publish(ctx, request); err != nil {
		slog.WarnContext(ctx, "signup: request not published", "event", request.EventName(), "error", err)
	}
	return SignUpResult{Browser: browser}, nil
}

// surveyDue reports whether a new User owes the Sign-up survey (SignUpConfig.SurveyDue); a
// config without the check owes none.
func (m *Module) surveyDue() bool {
	return m.signUp.SurveyDue != nil && m.signUp.SurveyDue()
}

// lookupUser finds the User that holds an address, whatever its case; ok is false when none.
func (m *Module) lookupUser(ctx context.Context, email string) (*ent.User, bool, error) {
	usr, err := m.users.UserByEmail(ctx, email)
	if ent.IsNotFound(err) {
		return nil, false, nil
	}
	return usr, err == nil, err
}

// signUpUser is the User a Sign-up with verification required ends on: a new Unverified User, or
// the one that holds the address with the password replaced while it is Unverified. Every path
// writes once, so the work does not say which one it took.
func (m *Module) signUpUser(ctx context.Context, n users.NewUser) (*ent.User, error) {
	email := n.Email
	usr, found, err := m.lookup(ctx, email)
	if err != nil {
		return nil, err
	}
	if !found {
		usr, err = m.users.CreateUnverified(ctx, n)
		if !errors.Is(err, users.ErrEmailTaken) {
			return usr, err
		}
		// Two Sign-ups of one new address raced: the loser finds the winner's User and goes on
		// as a repeated Sign-up, replacing the winner's password too (ADR 0030).
		usr, found, err = m.lookup(ctx, email)
		if err != nil {
			return nil, err
		}
		if !found {
			return nil, fmt.Errorf("signup: %q is taken but holds no User", email)
		}
	}
	return m.users.ReplaceUnverifiedPassword(ctx, usr.ID, n.PasswordHash, n.Consents)
}

// signUpVerified is a Sign-up on an Instance that does not require email verification: the
// User is Verified with its first Workspace at once, and the Sign-up completes as it does after
// a verification link (events.UserSignedUp, which welcomes the User). No session starts; the
// User signs in next. An address that belongs to a User is users.ErrEmailTaken, and nothing is
// created: with no mail to tell the owner, the Taken address is the answer (ADR 0031's one
// exception).
func (m *Module) signUpVerified(ctx context.Context, n users.NewUser, password string) (SignUpResult, error) {
	hash, err := m.passwords.Hash(password)
	if err != nil {
		return SignUpResult{}, err
	}
	n.PasswordHash = hash
	if _, err := m.createVerified(ctx, n, signedUp); err != nil {
		return SignUpResult{}, err
	}
	return SignUpResult{}, nil
}

// newBrowser draws the random value that marks one Sign-up browser and returns it with the
// hash that goes into the verification token.
func newBrowser() (value, hash string) {
	value = rand.Text()
	return value, users.BrowserHash(value)
}
