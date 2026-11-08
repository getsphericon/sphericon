package users

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/consentrecord"
	"github.com/getsphericon/sphericon/ent/invitation"
	"github.com/getsphericon/sphericon/ent/membership"
	"github.com/getsphericon/sphericon/ent/user"
	"github.com/getsphericon/sphericon/internal/domain/identity/consentrecords"
	"github.com/getsphericon/sphericon/internal/domain/identity/invitations"
	"github.com/getsphericon/sphericon/internal/domain/identity/memberships"
	"github.com/getsphericon/sphericon/internal/infra/db"
	"github.com/getsphericon/sphericon/internal/infra/events"
	"github.com/getsphericon/sphericon/internal/kernel/credentials"
	"github.com/getsphericon/sphericon/internal/kernel/i18n"
	"github.com/getsphericon/sphericon/internal/kernel/validation"
)

// The acceptance of an Invitation (ADR 0030, ADR 0062) makes or completes the User the
// Invitation names, so it is this module's; the Invitation itself is internal/domain/identity/invitations'.

var (
	// ErrSignInRequired means the invited address belongs to a Verified User and no
	// session was presented: only that User's own session may accept.
	ErrSignInRequired = validation.Unauthorized("users: sign in to accept this invitation", i18n.ErrorsInvitationSignInRequired)
	// ErrWrongAccount means the presented session is another User's than the
	// Verified User the invitation is for.
	ErrWrongAccount = validation.Denied("users: this invitation is for another account", i18n.ErrorsInvitationOtherAccount)
)

// credentialsRequired refuses accepting an Invitation for an address with no Verified User
// without a name and a password: the invitee sets up the account, so both are required.
func credentialsRequired() *validation.FieldsError {
	return &validation.FieldsError{Fields: map[string][]string{
		validation.FieldName:     {i18n.T(i18n.ErrorsNameRequired, nil)},
		validation.FieldPassword: {i18n.T(i18n.ErrorsPasswordRequired, nil)},
	}}
}

// InvitationAcceptance is what the person who follows an Invitation link gives.
type InvitationAcceptance struct {
	// Name and Password set up the account of an address no Verified User holds.
	Name     string
	Password string
	// SignupItems is that account's answers to the Sign-up items (ADR 0062).
	SignupItems []consentrecords.Answer
	// SessionEmail is the login email of the session the request carried ("" for none).
	SessionEmail string
}

// AcceptInvitation turns a pending Invitation into a Membership. A Verified User is
// attached only with their own session (ErrSignInRequired without one, ErrWrongAccount
// for another User's), decided on the row read inside the transaction, and is asked nothing
// more: consent follows the User, not the Workspace (ADR 0062). For an address with no User
// or an Unverified User (ADR 0030) the name and password are required (a *validation.FieldsError naming both
// otherwise) and the Sign-up items are answered as on the sign-up form (a
// *validation.FieldsError refuses the answers): the link proves control of the address, so
// the User is created, or the Unverified User's row completed (name and password set,
// Verified email stamped, session epoch bumped, `user.email_verify` audited), and nothing
// the person who signed up set survives but their Consent records, which are only added to:
// the answers are recorded after them with the source Invitation, and the latest per item is
// the current one. If the address was verified meanwhile that is ErrSignInRequired too
// (wrapping ErrAlreadyVerified): the address now has a Verified User, who signs in to accept. The
// Membership is created (a duplicate is not an error), the Invitation marked accepted and
// the acceptance audited, all in one transaction.
func (a *Module) AcceptInvitation(ctx context.Context, inv *ent.Invitation, in InvitationAcceptance) error {
	// The answers are checked before the transaction, which says on the row it reads whether
	// they are asked at all.
	consents, err := a.consents.Check(ctx, in.SignupItems, consentrecord.SourceInvitation)
	refused, itemsRefused := errors.AsType[*validation.FieldsError](err)
	if err != nil && !itemsRefused {
		return err
	}
	account := newAccount{name: in.Name, password: in.Password, consents: consents, refused: refused}
	err = a.bus.WithinTx(ctx, func(tx *ent.Client, pub events.Publisher) error {
		return a.acceptInvitationIn(ctx, tx, pub, inv, account, in.SessionEmail)
	})
	if errors.Is(err, ErrAlreadyVerified) {
		return fmt.Errorf("%w: %w", ErrSignInRequired, err)
	}
	return err
}

// acceptInvitationIn is AcceptInvitation in the caller's transaction tx, publishing on pub. account
// sets up the account of an address no Verified User holds; a caller that accepts for a Verified
// User by its proof of the address (sessionEmail) passes none.
func (a *Module) acceptInvitationIn(ctx context.Context, tx *ent.Client, pub events.Publisher, inv *ent.Invitation, account newAccount, sessionEmail string) error {
	at := a.now()
	role := membership.Role(inv.Role)
	u, proven, err := invitee(ctx, tx, a.passwords, inv, account, sessionEmail, at)
	if err != nil {
		return err
	}

	ts := tx.Scoped(inv.WorkspaceID)
	actor := events.Actor{Kind: events.ActorUser, ID: strconv.FormatInt(u.ID, 10), Name: u.Name}
	existing, err := ensureMembership(ctx, ts, u, role)
	if err != nil {
		return err
	}
	if existing != nil && memberships.IsMember(existing.Role) && memberships.IsAdmin(role) {
		// An explicit Invitation raises a member to admin; it never lowers a Role.
		if err := raiseToAdmin(ctx, ts, pub, inv, actor, u, existing); err != nil {
			return err
		}
	}

	if proven.now {
		// The address was proven now, so the domain-join step runs after the explicit
		// Invitation's Membership (which then wins: a Workspace the Invitation names is
		// not joined twice). Accepting an Invitation is untouched by the sign-up gate.
		if _, err := ApplyDomainJoinIn(ctx, tx, pub, u); err != nil {
			return err
		}
	}

	if err := invitations.MarkAcceptedIn(ctx, ts, inv.ID, at); err != nil {
		return err
	}
	if proven.byInvitation {
		if err := RecordActionOnUserIn(ctx, pub, actor, u, events.ActionUserEmailVerify, nil, inv.WorkspaceID); err != nil {
			return err
		}
	}
	return events.RecordAudit(ctx, pub, &events.AuditEntry{
		WorkspaceID: inv.WorkspaceID,
		Actor:       actor,
		Action:      events.ActionInvitationAccept,
		TargetType:  invitation.Label,
		TargetID:    strconv.FormatInt(inv.ID, 10),
		TargetName:  inv.Email,
		Diff:        map[string]any{invitation.FieldRole: events.Became(string(inv.Role))},
	})
}

// newAccount is what sets up the account of an invited address no Verified User holds: the name,
// the password, and the Consent records of the answers to the Sign-up items or their refusal.
type newAccount struct {
	name     string
	password string
	consents consentrecords.Consents
	refused  *validation.FieldsError
}

// acceptProof says how accepting an Invitation proved the address.
type acceptProof struct {
	// now is true when the address was proven by this acceptance (a stamp from empty).
	now bool
	// byInvitation is true when the Invitation link itself was the proof: the User was created or
	// completed from it.
	byInvitation bool
}

// invitee resolves the User an Invitation is accepted by: a Verified User by their own session,
// any other address by the account the link's holder sets up (ADR 0030), with the Consent
// records of its answers (ADR 0062).
func invitee(ctx context.Context, tx *ent.Client, passwords *credentials.Hasher, inv *ent.Invitation, account newAccount, sessionEmail string, at time.Time) (*ent.User, acceptProof, error) {
	u, err := tx.User.Query().Where(user.EmailEqualFold(inv.Email)).Only(ctx)
	if err != nil && !ent.IsNotFound(err) {
		return nil, acceptProof{}, err
	}
	if err == nil && u.EmailVerifiedAt != nil {
		now, err := proveByOwnSession(ctx, tx, u, inv.Email, sessionEmail, at)
		return u, acceptProof{now: now}, err
	}
	name := strings.TrimSpace(account.name)
	if name == "" || account.password == "" {
		return nil, acceptProof{}, credentialsRequired()
	}
	if account.refused != nil {
		return nil, acceptProof{}, account.refused
	}
	if err := CheckName(name); err != nil {
		return nil, acceptProof{}, err
	}
	hash, herr := passwords.Hash(account.password)
	if herr != nil {
		return nil, acceptProof{}, herr
	}
	if ent.IsNotFound(err) {
		err = constraints.Write(ctx, tx, func() error {
			var cerr error
			u, cerr = tx.User.Create().
				SetName(name).
				SetEmail(inv.Email).
				SetPasswordHash(hash).
				SetEmailVerifiedAt(at).
				SetEmailProvenAt(at).
				Save(ctx)
			return cerr
		})
	} else {
		u, err = MarkVerifiedIn(ctx, tx, u.ID, name, &hash, at)
		if err == nil {
			// The person who signed up is an invitee now, not a public Sign-up.
			err = clearSurveyDue(ctx, tx, u.ID)
		}
	}
	if err != nil {
		return nil, acceptProof{}, err
	}
	if err := consentrecords.Write(ctx, tx, u.ID, account.consents); err != nil {
		return nil, acceptProof{}, err
	}
	return u, acceptProof{now: true, byInvitation: true}, nil
}

// proveByOwnSession accepts for a Verified User: their own session is the proof of the address
// (ADR 0030). The invitation reached the address, and its owner's session accepted it. Only a
// stamp from empty is a proof (rows changed): an already proven address triggers nothing about
// domain join.
func proveByOwnSession(ctx context.Context, tx *ent.Client, u *ent.User, invitedEmail, sessionEmail string, at time.Time) (bool, error) {
	if sessionEmail == "" {
		return false, ErrSignInRequired
	}
	if !strings.EqualFold(sessionEmail, invitedEmail) {
		return false, ErrWrongAccount
	}
	n, err := tx.User.Update().Where(user.ID(u.ID), user.EmailProvenAtIsNil()).SetEmailProvenAt(at).Save(ctx)
	return n > 0, err
}

// ensureMembership returns the User's Membership in the scoped Workspace, creating it with role
// when there is none.
func ensureMembership(ctx context.Context, ts *ent.Scoped, u *ent.User, role membership.Role) (*ent.Membership, error) {
	existing, err := ts.Membership().Query().Where(membership.UserID(u.ID)).Only(ctx)
	if !ent.IsNotFound(err) {
		return existing, err
	}
	// DO NOTHING rather than catching a unique violation, which would abort the
	// transaction. A concurrent accept that inserted first leaves nothing to write
	// (db.Inserted); its Membership is read back, so the rules below still apply.
	err = memberships.AddInvitedIn(ctx, ts, u.ID, role)
	inserted, err := db.Inserted(err)
	switch {
	case err != nil:
		return nil, err
	case !inserted:
		return ts.Membership().Query().Where(membership.UserID(u.ID)).Only(ctx)
	}
	// Created: nothing was there before, so no earlier Membership to compare with.
	return nil, nil
}

// raiseToAdmin makes a member an admin and audits the change.
func raiseToAdmin(ctx context.Context, ts *ent.Scoped, pub events.Publisher, inv *ent.Invitation, actor events.Actor, u *ent.User, existing *ent.Membership) error {
	if err := memberships.SetRoleIn(ctx, ts, existing.ID, membership.RoleAdmin); err != nil {
		return err
	}
	return events.RecordAudit(ctx, pub, &events.AuditEntry{
		WorkspaceID: inv.WorkspaceID,
		Actor:       actor,
		Action:      events.ActionMembershipUpdate,
		TargetType:  membership.Label,
		TargetID:    strconv.FormatInt(existing.ID, 10),
		TargetName:  u.Email,
		Diff:        map[string]any{membership.FieldRole: events.Change(string(existing.Role), string(membership.RoleAdmin))},
	})
}
