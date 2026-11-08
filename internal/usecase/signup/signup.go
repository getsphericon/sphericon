package signup

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"log/slog"
	"maps"
	"strings"

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

// Consents answers a form's answers against the Sign-up items in effect: the Consent records
// they make, or the *validation.FieldsError that refuses them (consentrecords.Module.Check).
type Consents interface {
	Check(ctx context.Context, answers []consentrecords.Answer, source consentrecord.Source) (consentrecords.Consents, error)
}

// SignUpConfig is what a Sign-up needs besides the users module and the password hasher.
type SignUpConfig struct {
	Consents Consents
	Budget   Budget
	// RequireVerification is the email verification gate (ADR 0030): on in the saas Mode.
	RequireVerification bool
	// SurveyDue reports whether a new User owes the Sign-up survey (ADR 0055).
	SurveyDue func() bool
	// Lookup finds the User that holds an address, whatever its case; ok is false when none. Nil
	// reads it through the users module; a test stands in for it to replay the race of two
	// Sign-ups of one new address (step 3 of SignUp).
	Lookup func(ctx context.Context, email string) (u *ent.User, ok bool, err error)
}

// signUpForm is what a Sign-up checks beyond the contract, the name and email already trimmed.
type signUpForm struct {
	Name     string `json:"name" validate:"required,display_name"`
	Email    string `json:"email" validate:"required,account_email"`
	Password string `json:"password" validate:"required,password_not_blank,password_length"`
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
func (u *UseCase) SignUp(ctx context.Context, in SignUpInput) (SignUpResult, error) {
	cfg := u.signUp
	name := strings.TrimSpace(in.Name)
	email := strings.TrimSpace(in.Email)
	// The password is kept as sent: sign-in compares it as typed.
	fields, err := validation.Fields(signUpForm{Name: name, Email: email, Password: in.Password})
	if err != nil {
		return SignUpResult{}, err
	}
	consents, err := cfg.Consents.Check(ctx, in.SignupItems, consentrecord.SourceSignUpForm)
	refused, itemsRefused := errors.AsType[*validation.FieldsError](err)
	if err != nil && !itemsRefused {
		return SignUpResult{}, err
	}
	if fields != nil || itemsRefused {
		return SignUpResult{}, merged(fields, refused)
	}
	if !cfg.RequireVerification {
		return u.signUpVerified(ctx, users.NewUser{Name: name, Email: email, SurveyDue: cfg.SurveyDue(), Consents: consents}, in.Password)
	}

	granted, err := cfg.Budget.Take(ctx, email)
	if err != nil {
		return SignUpResult{}, err
	}
	// Hashed whatever the address is, so the work does not say whether it is Taken; a
	// Verified User's hash is dropped unused.
	hash, err := u.passwords.Hash(in.Password)
	if err != nil {
		return SignUpResult{}, err
	}
	usr, err := u.signUpUser(ctx, users.NewUser{Name: name, Email: email, PasswordHash: hash, SurveyDue: cfg.SurveyDue(), Consents: consents})
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
	if err := u.bus.Publish(ctx, request); err != nil {
		slog.WarnContext(ctx, "signup: request not published", "event", request.EventName(), "error", err)
	}
	return SignUpResult{Browser: browser}, nil
}

// lookupUser finds the User that holds an address, whatever its case; ok is false when none.
func (u *UseCase) lookupUser(ctx context.Context, email string) (*ent.User, bool, error) {
	usr, err := u.users.UserByEmail(ctx, email)
	if ent.IsNotFound(err) {
		return nil, false, nil
	}
	return usr, err == nil, err
}

// signUpUser is the User a Sign-up with verification required ends on: a new Unverified User, or
// the one that holds the address with the password replaced while it is Unverified. Every path
// writes once, so the work does not say which one it took.
func (u *UseCase) signUpUser(ctx context.Context, n users.NewUser) (*ent.User, error) {
	email := n.Email
	usr, found, err := u.signUp.Lookup(ctx, email)
	if err != nil {
		return nil, err
	}
	if !found {
		usr, err = u.users.CreateUnverified(ctx, n)
		if !errors.Is(err, users.ErrEmailTaken) {
			return usr, err
		}
		// Two Sign-ups of one new address raced: the loser finds the winner's User and goes on
		// as a repeated Sign-up, replacing the winner's password too (ADR 0030).
		usr, found, err = u.signUp.Lookup(ctx, email)
		if err != nil {
			return nil, err
		}
		if !found {
			return nil, fmt.Errorf("signup: %q is taken but holds no User", email)
		}
	}
	return u.users.ReplaceUnverifiedPassword(ctx, usr.ID, n.PasswordHash, n.Consents)
}

// signUpVerified is a Sign-up on an Instance that does not require email verification: the
// User is Verified with its first Workspace at once, and the Sign-up completes as it does after
// a verification link (events.UserSignedUp, which welcomes the User). No session starts; the
// User signs in next. An address that belongs to a User is users.ErrEmailTaken, and nothing is
// created: with no mail to tell the owner, the Taken address is the answer (ADR 0031's one
// exception).
func (u *UseCase) signUpVerified(ctx context.Context, n users.NewUser, password string) (SignUpResult, error) {
	hash, err := u.passwords.Hash(password)
	if err != nil {
		return SignUpResult{}, err
	}
	n.PasswordHash = hash
	if _, err := u.createVerified(ctx, n, signedUp); err != nil {
		return SignUpResult{}, err
	}
	return SignUpResult{}, nil
}

// merged is the one refusal of a form whose fields and Sign-up items' answers both may fail.
func merged(fields map[string][]string, items *validation.FieldsError) *validation.FieldsError {
	all := map[string][]string{}
	maps.Copy(all, fields)
	if items != nil {
		maps.Copy(all, items.Fields)
	}
	return &validation.FieldsError{Fields: all}
}

// newBrowser draws the random value that marks one Sign-up browser and returns it with the
// hash that goes into the verification token.
func newBrowser() (value, hash string) {
	value = rand.Text()
	return value, users.BrowserHash(value)
}
