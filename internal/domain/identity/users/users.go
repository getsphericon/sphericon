// Package users owns the User (ADR 0102): its credentials, the proof of its address (the
// verification and the other mailed links), its Linked identities and the Pending sign-up of a
// Sign-in provider, the sign-in decision, its profile, the acceptance gate of its requests (the
// Two-factor grace is internal/domain/identity/secondfactors'), the refusals of its deletion and
// the removal of its row (User deletion itself is the use case internal/usecase/userdeletion), the
// acceptance of an Invitation, which makes or completes the User the Invitation names, and the
// domain-join step of a proven address (ADR 0051). It sits above workspaces, memberships and
// invitations in identity.
//
// The module takes the raw client: a User belongs to no Workspace, and the actors it serves (a
// sign-up, a link, a provider callback, an Invitation's token) have no Workspace known up front
// (ADR 0017). The steps that make a User Verified create its first Workspace on the same
// transaction, so they are the use case internal/usecase/workspacecreation, which calls the
// ...In functions here for the User's part (ADR 0102).
package users

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"golang.org/x/text/language"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/consentrecord"
	"github.com/getsphericon/sphericon/ent/membership"
	"github.com/getsphericon/sphericon/ent/user"
	"github.com/getsphericon/sphericon/internal/domain/identity/consentrecords"
	"github.com/getsphericon/sphericon/internal/infra/db"
	"github.com/getsphericon/sphericon/internal/infra/events"
	"github.com/getsphericon/sphericon/internal/infra/throttle"
	"github.com/getsphericon/sphericon/internal/kernel/credentials"
	"github.com/getsphericon/sphericon/internal/kernel/i18n"
	"github.com/getsphericon/sphericon/internal/kernel/validation"
)

// Module reads and writes Users over the raw client.
type Module struct {
	ent       *ent.Client
	bus       *events.Bus
	passwords *credentials.Hasher
	consents  Consents
	// now is the Instance clock an address is verified and proven by, and a Sign-up ages by.
	now func() time.Time
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
// Sign-up items an account set up from an Invitation is asked (ADR 0062). now is the Instance clock.
func New(client *ent.Client, bus *events.Bus, passwords *credentials.Hasher, consents Consents, now func() time.Time, opts ...Option) *Module {
	a := &Module{ent: client, bus: bus, passwords: passwords, consents: consents, now: now}
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
	return UserByEmailIn(ctx, a.ent, email)
}

// UserByEmailIn is UserByEmail in the caller's transaction: a Staff member addresses a User by
// email at the command line (the Second factor reset).
func UserByEmailIn(ctx context.Context, tx *ent.Client, email string) (*ent.User, error) {
	return tx.User.Query().Where(user.Email(throttle.NormalizeEmail(email))).Only(ctx)
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
// address is verified from the Sign-up browser, or replaced on the link. A taken address is
// ErrEmailTaken. SurveyDue marks the User as owing the Sign-up survey (the Instance asks it).
func (a *Module) CreateUnverified(ctx context.Context, n NewUser) (*ent.User, error) {
	if err := CheckName(n.Name); err != nil {
		return nil, err
	}
	var created *ent.User
	err := a.bus.WithinTx(ctx, func(tx *ent.Client, _ events.Publisher) error {
		err := constraints.Write(ctx, tx, func() error {
			var err error
			created, err = tx.User.Create().SetName(n.Name).SetEmail(n.Email).SetPasswordHash(n.PasswordHash).
				SetNillableSignupSurveyDueAt(surveyDueAt(n.SurveyDue, a.now())).Save(ctx)
			return err
		})
		if err != nil {
			return err
		}
		return consentrecords.Write(ctx, tx, created.ID, n.Consents)
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
// User made by the platform itself rather than by a Sign-up. A taken address is ErrEmailTaken.
func (a *Module) CreateUser(ctx context.Context, name, email, passwordHash string) (*ent.User, error) {
	if err := CheckName(name); err != nil {
		return nil, err
	}
	var u *ent.User
	err := a.bus.WithinTx(ctx, func(tx *ent.Client, _ events.Publisher) error {
		return constraints.Write(ctx, tx, func() error {
			var err error
			u, err = tx.User.Create().SetName(name).SetEmail(email).SetPasswordHash(passwordHash).SetEmailVerifiedAt(a.now()).Save(ctx)
			return err
		})
	})
	return u, err
}

// MarkVerifiedIn completes an Unverified User's row: it stamps the Verified email, bumps the
// session epoch (ending every earlier session), sets the password hash when passwordHash is
// not nil (nil keeps the stored one) and the name when not empty. The write lands only on a still-Unverified User, so of two
// racing completions exactly one wins; the other gets ErrAlreadyVerified and changes
// nothing.
func MarkVerifiedIn(ctx context.Context, tx *ent.Client, id int64, name string, passwordHash *string, at time.Time) (*ent.User, error) {
	upd := tx.User.Update().
		Where(user.ID(id), user.EmailVerifiedAtIsNil()).
		SetEmailVerifiedAt(at).
		SetEmailProvenAt(at).
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

// ErrEmailTaken is an address that belongs to a User already: users_email_key refuses a second.
var ErrEmailTaken = validation.ConflictField(validation.FieldEmail, "users: the address belongs to a user", i18n.ErrorsEmailExists)

// constraints are the unique constraints of the Users, their Linked identities and the pending
// sign-ups (ADR 0106).
var constraints = db.Declare(
	db.Refusal("users_email_key", ErrEmailTaken),
	db.Refusal("useridentity_provider_subject", ErrIdentityInUse),
	db.Fault("pendingsignup_provider_subject", "HoldPending writes with ON CONFLICT DO NOTHING on it, after it deletes the account's pending sign-up"),
)

// FirstWorkspaceName is the fixed name of the Workspace made for a User at verification.
const FirstWorkspaceName = "Personal"

// CreateVerifiedIn writes, in tx, the User of a Sign-up on an install that does not require
// email verification (ADR 0030): Verified at once with its password, and the Consent records of
// its answers (ADR 0062). SurveyDue marks the User as owing the Sign-up survey. The caller has
// checked the name (CheckName) and creates the first Workspace on the same transaction
// (internal/usecase/workspacecreation). A taken address is ErrEmailTaken.
func CreateVerifiedIn(ctx context.Context, tx *ent.Client, n NewUser, at time.Time) (*ent.User, error) {
	var u *ent.User
	err := constraints.Write(ctx, tx, func() error {
		var err error
		u, err = tx.User.Create().
			SetName(n.Name).
			SetEmail(n.Email).
			SetPasswordHash(n.PasswordHash).
			SetEmailVerifiedAt(at).
			SetNillableSignupSurveyDueAt(surveyDueAt(n.SurveyDue, at)).
			Save(ctx)
		return err
	})
	if err != nil {
		return nil, err
	}
	return u, consentrecords.Write(ctx, tx, u.ID, n.Consents)
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
			n, err := tx.User.Update().Where(user.ID(id), user.EmailProvenAtIsNil()).SetEmailProvenAt(a.now()).Save(ctx)
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
			if _, err := ApplyDomainJoinIn(ctx, tx, pub, u); err != nil {
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

// ResetVerifiedPassword sets a Verified User's password from a reset link (ADR 0030): a password
// change recorded like ChangeProfile, which ends every earlier session. The link was mailed to the
// login address, so following it proves the address. An Unverified User's reset completes the
// Sign-up instead (internal/usecase/workspacecreation).
func (a *Module) ResetVerifiedPassword(ctx context.Context, id int64, passwordHash string) (*ent.User, error) {
	return a.updateProfile(ctx, id, profileWrite{PasswordHash: &passwordHash}, true)
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

// LockIn reads the User userID on the transaction tx and locks its row until the transaction
// ends, so a change of its session epoch or password waits for it.
func LockIn(ctx context.Context, tx *ent.Client, userID int64) (*ent.User, error) {
	return tx.User.Query().Where(user.ID(userID)).ForUpdate().Only(ctx)
}

// BumpSessionEpochIn ends the User's sessions on the transaction tx (a Passkey change).
func BumpSessionEpochIn(ctx context.Context, tx *ent.Client, userID int64) (*ent.User, error) {
	return tx.User.UpdateOneID(userID).AddSessionEpoch(1).Save(ctx)
}

// BumpSessionEpochFromIn ends the User's sessions on the transaction tx only while the User still
// has the session epoch and password of before, the snapshot a change of the User's Second factor
// checked (internal/domain/identity/secondfactors). bumped is false, and nothing is written, when
// either moved since.
func BumpSessionEpochFromIn(ctx context.Context, tx *ent.Client, before *ent.User) (saved *ent.User, bumped bool, err error) {
	update := tx.User.UpdateOneID(before.ID).Where(user.SessionEpochEQ(before.SessionEpoch))
	// A passwordless User stores no password as NULL or an empty optional string.
	if before.PasswordHash == "" {
		update.Where(user.Or(user.PasswordHashIsNil(), user.PasswordHashEQ("")))
	} else {
		update.Where(user.PasswordHashEQ(before.PasswordHash))
	}
	saved, err = update.AddSessionEpoch(1).Save(ctx)
	if ent.IsNotFound(err) {
		return nil, false, nil
	}
	return saved, err == nil, err
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
// (ADR 0051) in the same transaction. An address taken meanwhile is ErrEmailTaken.
func (a *Module) ChangeEmail(ctx context.Context, id int64, newEmail string) error {
	return a.bus.WithinTx(ctx, func(tx *ent.Client, pub events.Publisher) error {
		now := a.now()
		var u *ent.User
		err := constraints.Write(ctx, tx, func() error {
			var err error
			u, err = tx.User.UpdateOneID(id).SetEmail(newEmail).SetEmailVerifiedAt(now).SetEmailProvenAt(now).AddSessionEpoch(1).Save(ctx)
			return err
		})
		if err != nil {
			return err
		}
		_, err = ApplyDomainJoinIn(ctx, tx, pub, u)
		return err
	})
}
