// Package users owns the User (ADR 0102): its credentials, the proof of its address (the
// verification and the other mailed links), its Linked identities and the Pending sign-up of a
// Sign-in provider, the sign-in decision, its profile, the admission of its requests (the
// acceptance gate and the Two-factor grace), its deletion, the acceptance of an Invitation, which
// makes or completes the User the Invitation names, and the domain-join step of a proven address
// (ADR 0051). It sits above workspaces, memberships and invitations in identity.
//
// The module takes the raw client: a User belongs to no Workspace, and the actors it serves (a
// sign-up, a link, a provider callback, an Invitation's token) have no Workspace known up front
// (ADR 0017). The creation of a Workspace with its Owner Membership lives here until it becomes
// the use case workspacecreation (workspace_creation.go).
package users

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"net/mail"
	"strconv"
	"strings"
	"time"

	"golang.org/x/text/language"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/consentrecord"
	"github.com/getsphericon/sphericon/ent/membership"
	"github.com/getsphericon/sphericon/ent/user"
	"github.com/getsphericon/sphericon/internal/consentrecords"
	"github.com/getsphericon/sphericon/internal/infra/db"
	"github.com/getsphericon/sphericon/internal/infra/events"
	"github.com/getsphericon/sphericon/internal/infra/throttle"
	"github.com/getsphericon/sphericon/internal/kernel/credentials"
	"github.com/getsphericon/sphericon/internal/kernel/i18n"
	"github.com/getsphericon/sphericon/internal/kernel/validation"
	"github.com/getsphericon/sphericon/internal/kernel/workspaceid"
	"github.com/getsphericon/sphericon/internal/providerlimit"
	"github.com/getsphericon/sphericon/internal/signup"
)

// Module reads and writes Users over the raw client, and creates Workspaces.
type Module struct {
	ent         *ent.Client
	bus         *events.Bus
	passwords   *credentials.Hasher
	consents    Consents
	newPublicID func() string
	// sendCaps is whether the Instance has Send caps; only then does a new Workspace start with
	// the default defaultSendCap reads.
	sendCaps       providerlimit.SendCaps
	defaultSendCap DefaultSendCap
}

// Consents answers a form's answers against the Sign-up items in effect: the Consent records they
// make, or the *validation.FieldsError that refuses them (consentrecords.Module.Check), and says
// whether a User has a required acceptance pending (consentrecords.Module.Gated).
type Consents interface {
	Check(ctx context.Context, answers []consentrecords.Answer, source consentrecord.Source) (consentrecords.Consents, error)
	Gated(ctx context.Context, userID int64) (bool, error)
}

// Option tunes Module.
type Option func(*Module)

// New builds Module over the ent client; the bus opens the transactions that
// span several writes, passwords hashes the passwords a User sets, and consents answers the
// Sign-up items an account set up from an Invitation is asked (ADR 0062).
func New(client *ent.Client, bus *events.Bus, passwords *credentials.Hasher, consents Consents, opts ...Option) *Module {
	a := &Module{ent: client, bus: bus, passwords: passwords, consents: consents, newPublicID: workspaceid.New}
	for _, opt := range opts {
		opt(a)
	}
	return a
}

// ErrMalformedRegionalFormat refuses a Regional format that is not a well-formed BCP 47 tag.
var ErrMalformedRegionalFormat = validation.RefuseField("regionalFormat", "users: malformed regional format", i18n.ErrorsRegionalFormatMalformed)

// canonicalRegionalFormat is the Regional format tag in its canonical spelling (`de-de` is
// `de-DE`), or ErrMalformedRegionalFormat when it is not a well-formed BCP 47 tag. The tag is not
// checked against a list of regions: the app's own list is a convenience, not a rule. Its length
// is the contract's (RegionalFormat), which the request validation holds.
func canonicalRegionalFormat(typed string) (string, error) {
	tag := strings.TrimSpace(typed)
	if tag == "" {
		return "", ErrMalformedRegionalFormat
	}
	parsed, err := language.Parse(tag)
	if err != nil {
		return "", fmt.Errorf("%w: %w", ErrMalformedRegionalFormat, err)
	}
	return parsed.String(), nil
}

// ErrEmailNotVerified refuses a User whose email is not proven yet (ADR 0030): creating a
// Workspace, so sign-up verification is not bypassed by creating Workspaces first, and the
// verification gate of the password login.
var ErrEmailNotVerified = validation.Unverified("users: the email is not verified", i18n.ErrorsEmailVerificationRequired)

// User loads a User by id.
func (a *Module) User(ctx context.Context, id int64) (*ent.User, error) {
	return a.ent.User.Get(ctx, id)
}

// UserByEmail loads a User by login email, whatever its case (an ent NotFound error when none):
// a stored email is lower-cased (ent/schema LowerEmail).
func (a *Module) UserByEmail(ctx context.Context, email string) (*ent.User, error) {
	return a.ent.User.Query().Where(user.Email(throttle.NormalizeEmail(email))).Only(ctx)
}

// UserByLoginAddress loads a User by a normalized login address, ignoring case (an ent
// NotFound error when none): the sign-in address is lower-cased once by the login module,
// and a stored email is lower-cased on write (ent/schema LowerEmail).
func (a *Module) UserByLoginAddress(ctx context.Context, address string) (*ent.User, error) {
	return a.ent.User.Query().Where(user.EmailEqualFold(address)).Order(ent.Asc(user.FieldID)).First(ctx)
}

// EmailTaken reports whether an account already uses the login email, whatever its case.
func (a *Module) EmailTaken(ctx context.Context, email string) (bool, error) {
	return a.ent.User.Query().Where(user.Email(throttle.NormalizeEmail(email))).Exist(ctx)
}

// ErrNameInvalid means a User's name is one validation.DisplayName refuses: too long, empty to
// the eye, or holding a character that breaks a line or hides.
var ErrNameInvalid = errors.New("users: the name is not a valid display name")

// CheckName checks every way a person names a User. A Sign-in provider's invalid name is
// dropped instead (providerName).
func CheckName(name string) error {
	if !validation.DisplayName(name) {
		return fmt.Errorf("%w: %w", ErrNameInvalid, validation.Refused(validation.FieldName,
			i18n.T(i18n.ErrorsNameInvalid, map[string]any{"NameMax": validation.NameMaxLength})))
	}
	return nil
}

// CreateUnverified creates the Unverified User of a Sign-up (ADR 0030): its name, email and
// password hash from the Sign-up form, no Membership, and in the same transaction the Consent
// records of its answers to the Sign-up items (ADR 0062). The password goes live only when the
// address is verified from the Sign-up browser, or replaced on the link. The unique index on the
// email refuses a taken address (see db.IsUniqueViolation). SurveyDue marks the User as owing the
// Sign-up survey (the Instance asks it).
func (a *Module) CreateUnverified(ctx context.Context, n signup.NewUser) (*ent.User, error) {
	if err := CheckName(n.Name); err != nil {
		return nil, err
	}
	var created *ent.User
	err := a.bus.WithinTx(ctx, func(tx *ent.Client, _ events.Publisher) error {
		u, err := tx.User.Create().SetName(n.Name).SetEmail(n.Email).SetPasswordHash(n.PasswordHash).
			SetNillableSignupSurveyDueAt(surveyDueAt(n.SurveyDue)).Save(ctx)
		if err != nil {
			return err
		}
		created = u
		return consentrecords.Write(ctx, tx, u.ID, n.Consents)
	})
	return created, err
}

// ReplaceUnverifiedPassword is a repeated Sign-up of an address (ADR 0030): it
// stores the new password hash on the User while it is still Unverified, which ends
// every verification link minted over the earlier hash, and adds the Consent records of the
// repeated Sign-up's answers in the same transaction. A Verified User is left
// untouched, records included: whoever signed up has not proven the address. It returns the User
// as it stands afterwards.
func (a *Module) ReplaceUnverifiedPassword(ctx context.Context, id int64, passwordHash string, consents consentrecords.Consents) (*ent.User, error) {
	err := a.bus.WithinTx(ctx, func(tx *ent.Client, _ events.Publisher) error {
		n, err := tx.User.Update().
			Where(user.ID(id), user.EmailVerifiedAtIsNil()).
			SetPasswordHash(passwordHash).
			Save(ctx)
		if err != nil || n == 0 {
			return err
		}
		return consentrecords.Write(ctx, tx, id, consents)
	})
	if err != nil {
		return nil, err
	}
	return a.ent.User.Get(ctx, id)
}

// CreateUser creates a User whose email is already proven, with a password hash: a
// User made by the platform itself rather than by a Sign-up.
func (a *Module) CreateUser(ctx context.Context, name, email, passwordHash string) (*ent.User, error) {
	if err := CheckName(name); err != nil {
		return nil, err
	}
	var u *ent.User
	err := a.bus.WithinTx(ctx, func(tx *ent.Client, _ events.Publisher) error {
		var err error
		u, err = tx.User.Create().SetName(name).SetEmail(email).SetPasswordHash(passwordHash).SetEmailVerifiedAt(time.Now()).Save(ctx)
		return err
	})
	return u, err
}

// markVerified completes an Unverified User's row: it stamps the Verified email, bumps the
// session epoch (ending every earlier session), sets the password hash when passwordHash is
// not nil (nil keeps the stored one) and the name when not empty. The write lands only on a still-Unverified User, so of two
// racing completions exactly one wins; the other gets ErrAlreadyVerified and changes
// nothing.
func markVerified(ctx context.Context, tx *ent.Client, id int64, name string, passwordHash *string) (*ent.User, error) {
	upd := tx.User.Update().
		Where(user.ID(id), user.EmailVerifiedAtIsNil()).
		SetEmailVerifiedAt(time.Now()).
		SetEmailProvenAt(time.Now()).
		AddSessionEpoch(1)
	if passwordHash != nil {
		upd = upd.SetPasswordHash(*passwordHash)
	}
	if name != "" {
		upd = upd.SetName(name)
	}
	n, err := upd.Save(ctx)
	if err != nil {
		return nil, err
	}
	if n == 0 {
		return nil, ErrAlreadyVerified
	}
	return tx.User.Get(ctx, id)
}

// ErrAlreadyVerified means the User's email was already verified: a link that
// completes an Unverified User lost the race to another one.
var ErrAlreadyVerified = errors.New("users: the email is already verified")

// FirstWorkspaceName is the fixed name of the Workspace made for a User at verification.
const FirstWorkspaceName = "Personal"

// VerifyEmail completes a Sign-up from its link (ADR 0030), in one transaction:
// it stamps the Verified email, sets the password hash to *passwordHash (nil keeps the one
// stored at Sign-up), bumps the session epoch (ending every session issued before, as
// a password change does) and creates the User's first Workspace, named FirstWorkspaceName,
// with the User as owner. It records `user.email_verify` in that Workspace's Audit log and
// returns the User. The address was just proven, so it is a domain-join step (ADR 0051): a
// verified Join domain equal to its domain, with the switch on, makes the User a `member`
// of that Workspace before the first Workspace is created. landing is where the User goes
// next: the joined Workspace, else the first one.
// ErrAlreadyVerified when the User is no longer Unverified (a used link).
func (a *Module) VerifyEmail(ctx context.Context, id int64, passwordHash *string) (u *ent.User, landing *ent.Workspace, err error) {
	return a.verified(ctx, provenNow, func(tx *ent.Client, _ events.Publisher) (*ent.User, error) {
		return markVerified(ctx, tx, id, "", passwordHash)
	})
}

// SignUpVerified is a Sign-up on an install that does not require email
// verification (ADR 0030): in one transaction it creates a User that is Verified at once with
// its password and the Consent records of its answers (ADR 0062), and the first Workspace
// (FirstWorkspaceName), exactly as VerifyEmail does. The
// unique index on the email refuses a taken address (see db.IsUniqueViolation). SurveyDue marks
// the User as owing the Sign-up survey.
func (a *Module) SignUpVerified(ctx context.Context, n signup.NewUser) (*ent.User, error) {
	if err := CheckName(n.Name); err != nil {
		return nil, err
	}
	u, _, err := a.verified(ctx, noJoin, func(tx *ent.Client, _ events.Publisher) (*ent.User, error) {
		u, err := tx.User.Create().
			SetName(n.Name).
			SetEmail(n.Email).
			SetPasswordHash(n.PasswordHash).
			SetEmailVerifiedAt(time.Now()).
			SetNillableSignupSurveyDueAt(surveyDueAt(n.SurveyDue)).
			Save(ctx)
		if err != nil {
			return nil, err
		}
		return u, consentrecords.Write(ctx, tx, u.ID, n.Consents)
	})
	return u, err
}

// CreateOwner is the Instance operator's way to make an Owner from a shell (ADR 0042): a
// Verified User with a generated password and its first
// Workspace, named FirstWorkspaceName, exactly as SignUpVerified makes them. The password
// is returned once; an address that does not parse is refused, and the unique index on the
// email refuses a taken one (see db.IsUniqueViolation).
func (a *Module) CreateOwner(ctx context.Context, email, name string) (password string, err error) {
	if _, err := mail.ParseAddress(email); err != nil {
		return "", fmt.Errorf("%q is not an email address", email)
	}
	password = rand.Text()
	hash, err := a.passwords.Hash(password)
	if err != nil {
		return "", err
	}
	if _, err := a.SignUpVerified(ctx, signup.NewUser{Name: name, Email: email, PasswordHash: hash}); err != nil {
		return "", err
	}
	return password, nil
}

// provenNow and noJoin say whether a step that makes a User Verified also proved the address
// (stamped email_proven_at from empty), which makes it a domain-join step (ADR 0051). An
// Instance with verification off stamps only verified_at, which proves nothing.
const (
	provenNow = true
	noJoin    = false
)

// verified is what makes a User Verified, shared by VerifyEmail, SignUpVerified and the
// provider sign-in: write stores the User's verified state (and whatever else its step writes,
// on the same publisher); then, when proven (the step stamped email_proven_at from empty), the
// domain-join step runs; then the User's first Workspace is created and `user.email_verify`
// recorded in its Audit log, all in one transaction. landing is the Workspace the User goes to
// next: the one joined by domain, else the first.
func (a *Module) verified(ctx context.Context, proven bool, write func(tx *ent.Client, pub events.Publisher) (*ent.User, error)) (*ent.User, *ent.Workspace, error) {
	for {
		var verified *ent.User
		var landing *ent.Workspace
		written := false
		err := a.bus.WithinTx(ctx, func(tx *ent.Client, pub events.Publisher) error {
			u, err := write(tx, pub)
			if err != nil {
				return err
			}
			written = true
			var joined *ent.Workspace
			if proven {
				if joined, err = applyDomainJoin(ctx, tx, pub, u); err != nil {
					return err
				}
			}
			first, err := a.createWorkspaceIn(ctx, tx, u.ID, FirstWorkspaceName, a.newPublicID())
			if err != nil {
				return err
			}
			verified, landing = u, first
			if joined != nil {
				landing = joined
			}
			return RecordUserAction(ctx, tx, pub, u, events.ActionUserEmailVerify, nil)
		})
		if written && db.IsUniqueViolation(err) {
			continue // drew a taken id; the next attempt retries
		}
		return verified, landing, err
	}
}

// profileWrite is what updateProfile writes: a nil Name, PasswordHash, Timezone or RegionalFormat
// leaves that field unchanged, ClearTimezone clears the User time zone and ClearRegionalFormat
// the Regional format.
type profileWrite struct {
	Name, PasswordHash, Timezone, RegionalFormat *string
	ClearTimezone, ClearRegionalFormat           bool
}

// updateProfile writes w to the User and returns the stored User. A password change bumps the
// session epoch, ending every session issued before it (ADR 0020), and is recorded as an Audit
// entry in every Workspace the User belongs to, in the same transaction. proven also stamps the
// address as proven (a reset link was followed, which was mailed to it).
func (a *Module) updateProfile(ctx context.Context, id int64, w profileWrite, proven bool) (*ent.User, error) {
	if w.Name != nil {
		if err := CheckName(*w.Name); err != nil {
			return nil, err
		}
	}
	var saved *ent.User
	err := a.bus.WithinTx(ctx, func(tx *ent.Client, pub events.Publisher) error {
		// The mailed link proves the address; only a stamp from empty is a domain-join step,
		// so the write is guarded and its rows-changed count says whether it stamped (ADR 0051).
		stamped := false
		if proven {
			n, err := tx.User.Update().Where(user.ID(id), user.EmailProvenAtIsNil()).SetEmailProvenAt(time.Now()).Save(ctx)
			if err != nil {
				return err
			}
			stamped = n > 0
		}
		upd := tx.User.UpdateOneID(id).SetNillableName(w.Name).SetNillableTimezone(w.Timezone).SetNillableRegionalFormat(w.RegionalFormat)
		if w.ClearTimezone {
			upd = upd.ClearTimezone()
		}
		if w.ClearRegionalFormat {
			upd = upd.ClearRegionalFormat()
		}
		if w.PasswordHash != nil {
			// A new password ends every session issued before it (ADR 0020).
			upd = upd.SetPasswordHash(*w.PasswordHash).AddSessionEpoch(1)
		}
		u, err := upd.Save(ctx)
		if err != nil {
			return err
		}
		saved = u
		if stamped {
			if _, err := applyDomainJoin(ctx, tx, pub, u); err != nil {
				return err
			}
		}
		if w.PasswordHash == nil {
			return nil
		}
		return RecordUserAction(ctx, tx, pub, u, events.ActionUserPasswordChange, map[string]any{"password": "changed"})
	})
	return saved, err
}

// ResetPassword sets the User's password from a reset link (ADR 0030). The link was
// mailed to the login address, so following it proves the address as much as the
// verification link does: for an Unverified User it completes the Sign-up
// exactly as VerifyEmail (Verified email, first Workspace, `user.email_verify`) and
// reports completedSignUp = true with the User, so the caller sends the welcome
// email. For a Verified User it is only a password change, recorded like ChangeProfile.
// Either way every earlier session ends.
func (a *Module) ResetPassword(ctx context.Context, id int64, passwordHash string) (u *ent.User, completedSignUp bool, err error) {
	u, err = a.User(ctx, id)
	if err != nil {
		return nil, false, err
	}
	if u.EmailVerifiedAt == nil {
		u, _, err = a.VerifyEmail(ctx, id, &passwordHash)
		if err == nil {
			return u, true, nil
		}
		if !errors.Is(err, ErrAlreadyVerified) {
			return nil, false, err
		}
		// A concurrent link verified the User first: this is now a plain password change.
	}
	u, err = a.updateProfile(ctx, id, profileWrite{PasswordHash: &passwordHash}, true)
	return u, false, err
}

// EndSessions bumps the User's session epoch, ending every session issued before
// it on every device (ADR 0020, "sign out everywhere"), and records
// `user.sign_out_everywhere` in the same transaction.
func (a *Module) EndSessions(ctx context.Context, id int64) error {
	return a.bus.WithinTx(ctx, func(tx *ent.Client, pub events.Publisher) error {
		u, err := tx.User.UpdateOneID(id).AddSessionEpoch(1).Save(ctx)
		if err != nil {
			return err
		}
		return RecordUserAction(ctx, tx, pub, u, events.ActionUserSignOutEverywhere, nil)
	})
}

// LoginPassword is the method of a login by email and password in the `user.login` entry.
const LoginPassword = "password"

// LoginPasskey is the method of a login with a Passkey in the `user.login` entry (ADR 0052).
const LoginPasskey = "passkey"

// RecordLogin records a successful sign-in of the User as `user.login` in the log of
// every Workspace they belong to (ADR 0022: a login belongs to a User, not a
// Workspace, so there is no account-level log). The entry names the method: LoginPassword
// or the registry name of the Sign-in provider (ADR 0048).
func (a *Module) RecordLogin(ctx context.Context, u *ent.User, method string) error {
	return a.bus.WithinTx(ctx, func(tx *ent.Client, pub events.Publisher) error {
		return RecordUserAction(ctx, tx, pub, u, events.ActionUserLogin, map[string]any{"method": method})
	})
}

// RecordUserAction publishes one Audit entry per Workspace the User holds a
// Membership in, and none anywhere else. The User is the actor and the target.
func RecordUserAction(ctx context.Context, tx *ent.Client, pub events.Publisher, u *ent.User, action string, diff map[string]any) error {
	actor := events.Actor{Kind: events.ActorUser, ID: strconv.FormatInt(u.ID, 10), Name: u.Name}
	return RecordActionOnUser(ctx, tx, pub, actor, u, action, diff)
}

// RecordActionOnUser publishes action by actor on the User u into the log of every
// Workspace u holds a Membership in (a staff member acting on a User), and none
// anywhere else.
func RecordActionOnUser(ctx context.Context, tx *ent.Client, pub events.Publisher, actor events.Actor, u *ent.User, action string, diff map[string]any) error {
	held, err := tx.Membership.Query().Where(membership.UserID(u.ID)).All(ctx)
	if err != nil {
		return err
	}
	ids := make([]int64, len(held))
	for i, m := range held {
		ids[i] = m.WorkspaceID
	}
	return RecordActionOnUserIn(ctx, pub, actor, u, action, diff, ids...)
}

// RecordActionOnUserIn publishes action by actor on the User u into the logs of the
// given Workspaces only: an Owner or Admin acting on a member is recorded where they
// hold that authority, not in the member's other Workspaces.
func RecordActionOnUserIn(ctx context.Context, pub events.Publisher, actor events.Actor, u *ent.User, action string, diff map[string]any, workspaceIDs ...int64) error {
	id := strconv.FormatInt(u.ID, 10)
	for _, ws := range workspaceIDs {
		if err := events.RecordAudit(ctx, pub, &events.AuditEntry{
			WorkspaceID: ws,
			Actor:       actor,
			Action:      action,
			TargetType:  user.Label,
			TargetID:    id,
			TargetName:  u.Name,
			Diff:        diff,
		}); err != nil {
			return err
		}
	}
	return nil
}

// ChangeEmail swaps the login email for one proven by a confirmation link, so it is
// stored verified, and bumps the session epoch: every session issued before the
// change ends (ADR 0020). The new address is proven now, so it is a domain-join step
// (ADR 0051) in the same transaction. The unique index guards a race on the address (see
// db.IsUniqueViolation).
func (a *Module) ChangeEmail(ctx context.Context, id int64, newEmail string) error {
	return a.bus.WithinTx(ctx, func(tx *ent.Client, pub events.Publisher) error {
		now := time.Now()
		u, err := tx.User.UpdateOneID(id).SetEmail(newEmail).SetEmailVerifiedAt(now).SetEmailProvenAt(now).AddSessionEpoch(1).Save(ctx)
		if err != nil {
			return err
		}
		_, err = applyDomainJoin(ctx, tx, pub, u)
		return err
	})
}
