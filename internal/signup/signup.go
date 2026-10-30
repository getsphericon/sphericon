// Package signup is a person creating a User for themself with a name, an email and a
// password (the Sign-up, ADR 0030). Sign-up is open on every Instance (ADR 0063). It checks the
// form, and then, on an Instance that requires email verification, does the same work for every
// address so that nobody but its owner learns that an address is Taken (ADR 0031). It returns
// one Result and holds no database client: an adapter supplies the Store, the address's mail
// budget, the verification token and the mails, and turns the Result into the answer and the
// Sign-up browser's cookie.
//
// The order is the module's contract:
//
//  1. the form is checked before any work, so a refused one costs no hash;
//  2. with verification required, every address costs the same: a slot of its mail budget, a
//     password hash, a lookup, a write (the create, or the replace that a Verified User's row
//     ignores), a token and exactly one mail job, which delivers only when the budget granted
//     it. Only what the job carries differs: a verification link for a new or Unverified User,
//     a tokenless notice for a Verified one;
//  3. two Sign-ups of one new address may race on the unique email: the loser finds the
//     winner's User and goes on as a repeated Sign-up of it, so a race is no answer of its own;
//  4. a repeated Sign-up of an Unverified address replaces its password, which ends every
//     earlier link, and draws a new Sign-up browser;
//  5. with verification off there is no mail to tell the owner, so a Taken address is the
//     answer (the one exception, ADR 0031), and the User is Verified at once.
package signup

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"maps"
	"strings"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/consentrecord"
	"github.com/getsphericon/sphericon/internal/consentrecords"
	"github.com/getsphericon/sphericon/internal/credentials"
	"github.com/getsphericon/sphericon/internal/validation"
)

// ErrEmailTaken refuses a Sign-up of an address that belongs to a User, on an Instance that does
// not require email verification (ADR 0031's one exception): nothing was created.
var ErrEmailTaken = validation.ConflictField(validation.FieldEmail, "signup: the address belongs to a user", "errors.email_exists")

// ErrTaken is what a Store's create returns when the unique email refuses the address.
var ErrTaken = errors.New("signup: address taken")

// Input is the Sign-up form as typed.
type Input struct {
	Name     string
	Email    string
	Password string
	// SignupItems is the answers to the Sign-up items the form showed (ADR 0062).
	SignupItems []consentrecords.Answer
}

// NewUser is a User a Sign-up creates, with the Consent records its answers make, which the Store
// writes in the transaction that creates the User.
type NewUser struct {
	Name         string
	Email        string
	PasswordHash string
	// SurveyDue marks the User as owing the Sign-up survey.
	SurveyDue bool
	Consents  consentrecords.Consents
}

// Result is the answer of a Sign-up that went through. With verification required, Browser is
// the Sign-up browser's value, whatever the address; without, the User is Verified with its first
// Workspace and the welcome mail is queued.
type Result struct {
	// Browser is the random value of the Sign-up browser's cookie, for a Sign-up that a
	// verification link follows; empty otherwise. Its hash is what the link carries.
	Browser string
}

// Store is the adapter's side of the Users.
type Store interface {
	// Lookup finds the User that holds an address, whatever its case; ok is false when none.
	Lookup(ctx context.Context, email string) (u *ent.User, ok bool, err error)
	// CreateUnverified creates an Unverified User with the password hash and its Consent records
	// and nothing else, or returns ErrTaken when the unique email refuses the address.
	CreateUnverified(ctx context.Context, u NewUser) (*ent.User, error)
	// ReplaceUnverifiedPassword stores the hash and adds the Consent records on the User while it
	// is Unverified, which ends every link minted over the earlier hash; a Verified User is left
	// as it is, records included. It returns the User as it stands afterwards.
	ReplaceUnverifiedPassword(ctx context.Context, id int64, passwordHash string, consents consentrecords.Consents) (*ent.User, error)
	// CreateVerified creates a Verified User with its first Workspace and its Consent records, or
	// returns ErrTaken when the address is taken.
	CreateVerified(ctx context.Context, u NewUser) (*ent.User, error)
}

// Budget is the mail budget of an address (ADR 0031): Take grants one slot and reports
// whether it was granted. A request whose slot is refused still does all the work; its mail
// job is queued but delivers nothing.
type Budget interface {
	Take(ctx context.Context, address string) (granted bool, err error)
}

// Mail queues the mails a Sign-up sends. With send false a job is queued all the same but
// delivers nothing.
type Mail interface {
	EnqueueEmailVerification(ctx context.Context, email, token string, send bool) error
	EnqueueAddressTakenNotice(ctx context.Context, email string, send bool) error
	EnqueueWelcome(ctx context.Context, email, name string) error
}

// Consents answers a form's answers against the Sign-up items in effect: the Consent records
// they make, or the *validation.FieldsError that refuses them (consentrecords.Module.Check).
type Consents interface {
	Check(ctx context.Context, answers []consentrecords.Answer, source consentrecord.Source) (consentrecords.Consents, error)
}

// Config wires the module to its adapter.
type Config struct {
	Store    Store
	Consents Consents
	Budget   Budget
	Mail     Mail
	// Mint mints the verification token of a User, bound to its stored password hash and
	// carrying the hash of the Sign-up browser it trusts.
	Mint func(u *ent.User, browserHash string) (string, error)
	// Passwords hashes the password a Sign-up sets.
	Passwords *credentials.Hasher
	// RequireVerification is the email verification gate (ADR 0030): on in the saas Mode.
	RequireVerification bool
	// SurveyDue reports whether a new User owes the Sign-up survey (ADR 0055).
	SurveyDue func() bool
}

// Module is the Sign-up.
type Module struct{ cfg Config }

// New builds the module over its adapter.
func New(cfg Config) *Module { return &Module{cfg: cfg} }

// form is what a Sign-up checks beyond the contract, the name and email already trimmed.
type form struct {
	Name     string `json:"name" validate:"required"`
	Email    string `json:"email" validate:"required,account_email"`
	Password string `json:"password" validate:"required,password_not_blank,password_length"`
}

// SignUp runs a Sign-up. A refused form is a *validation.FieldsError naming every field, the
// Sign-up items' answers included (ADR 0062).
func (m *Module) SignUp(ctx context.Context, in Input) (Result, error) {
	name := strings.TrimSpace(in.Name)
	email := strings.TrimSpace(in.Email)
	// The password is kept as sent: sign-in compares it as typed.
	fields, err := validation.Fields(form{Name: name, Email: email, Password: in.Password})
	if err != nil {
		return Result{}, err
	}
	consents, err := m.cfg.Consents.Check(ctx, in.SignupItems, consentrecord.SourceSignUpForm)
	refused, itemsRefused := errors.AsType[*validation.FieldsError](err)
	if err != nil && !itemsRefused {
		return Result{}, err
	}
	if fields != nil || itemsRefused {
		return Result{}, merged(fields, refused)
	}
	if !m.cfg.RequireVerification {
		return m.signUpVerified(ctx, NewUser{Name: name, Email: email, SurveyDue: m.cfg.SurveyDue(), Consents: consents}, in.Password)
	}

	granted, err := m.cfg.Budget.Take(ctx, email)
	if err != nil {
		return Result{}, err
	}
	// Hashed whatever the address is, so the work does not say whether it is Taken; a
	// Verified User's hash is dropped unused.
	hash, err := m.cfg.Passwords.Hash(in.Password)
	if err != nil {
		return Result{}, err
	}
	u, err := m.user(ctx, NewUser{Name: name, Email: email, PasswordHash: hash, SurveyDue: m.cfg.SurveyDue(), Consents: consents})
	if err != nil {
		return Result{}, err
	}

	// The token is bound to the stored password hash, so it works until a Sign-up replaces the
	// hash or the address is verified, and it carries the hash of the Sign-up browser drawn
	// here. A Verified User gets the same token and browser, and its notice carries no token.
	browser, browserHash := newBrowser()
	token, err := m.cfg.Mint(u, browserHash)
	if err != nil {
		return Result{}, err
	}
	// Best-effort: a lost mail is asked for again.
	if u.EmailVerifiedAt != nil {
		if err := m.cfg.Mail.EnqueueAddressTakenNotice(ctx, u.Email, granted); err != nil {
			mailNotQueued(ctx, "AddressTakenNotice", err)
		}
	} else if err := m.cfg.Mail.EnqueueEmailVerification(ctx, u.Email, token, granted); err != nil {
		mailNotQueued(ctx, "EmailVerification", err)
	}
	return Result{Browser: browser}, nil
}

// user is the User a Sign-up with verification required ends on: a new Unverified User, or the
// one that holds the address with the password replaced while it is Unverified. Every path
// writes once, so the work does not say which one it took.
func (m *Module) user(ctx context.Context, n NewUser) (*ent.User, error) {
	email := n.Email
	u, found, err := m.cfg.Store.Lookup(ctx, email)
	if err != nil {
		return nil, err
	}
	if !found {
		u, err = m.cfg.Store.CreateUnverified(ctx, n)
		if !errors.Is(err, ErrTaken) {
			return u, err
		}
		// Two Sign-ups of one new address raced: the loser finds the winner's User and goes on
		// as a repeated Sign-up, replacing the winner's password too (ADR 0030).
		u, found, err = m.cfg.Store.Lookup(ctx, email)
		if err != nil {
			return nil, err
		}
		if !found {
			return nil, fmt.Errorf("signup: %q is taken but holds no User", email)
		}
	}
	return m.cfg.Store.ReplaceUnverifiedPassword(ctx, u.ID, n.PasswordHash, n.Consents)
}

// signUpVerified is a Sign-up on an Instance that does not require email verification: the
// User is Verified with its first Workspace at once, and the welcome mail follows as it does
// after a verification link. No session starts; the User signs in next.
func (m *Module) signUpVerified(ctx context.Context, n NewUser, password string) (Result, error) {
	hash, err := m.cfg.Passwords.Hash(password)
	if err != nil {
		return Result{}, err
	}
	n.PasswordHash = hash
	u, err := m.cfg.Store.CreateVerified(ctx, n)
	switch {
	case errors.Is(err, ErrTaken):
		return Result{}, ErrEmailTaken
	case err != nil:
		return Result{}, err
	}
	if err := m.cfg.Mail.EnqueueWelcome(ctx, u.Email, u.Name); err != nil {
		mailNotQueued(ctx, "Welcome", err)
	}
	return Result{}, nil
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

func mailNotQueued(ctx context.Context, mail string, err error) {
	slog.WarnContext(ctx, "mail not enqueued", "mail", mail, "error", err)
}

// newBrowser draws the random value that marks one Sign-up browser and returns it with the
// hash that goes into the verification token.
func newBrowser() (value, hash string) {
	value = rand.Text()
	return value, BrowserHash(value)
}

// BrowserHash is the hash of a Sign-up browser cookie's value, empty for no cookie.
func BrowserHash(value string) string {
	if value == "" {
		return ""
	}
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}

// SameBrowser reports whether a request's Sign-up browser cookie value is the one whose hash a
// verification token carries. A token minted without a hash (a link asked for from a browser
// that holds no cookie) matches no browser.
func SameBrowser(tokenHash, cookieValue string) bool {
	if tokenHash == "" || cookieValue == "" {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(tokenHash), []byte(BrowserHash(cookieValue))) == 1
}
