package accounts

import (
	"context"
	"errors"
	"slices"
	"strings"
	"time"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/user"
	"github.com/getsphericon/sphericon/ent/useridentity"
	"github.com/getsphericon/sphericon/internal/db"
	"github.com/getsphericon/sphericon/internal/events"
	"github.com/getsphericon/sphericon/internal/signin"
)

// ProviderLogin is a provider's answer, ready for the sign-in decision (ADR 0048).
type ProviderLogin struct {
	// Provider is the registry name; Subject the provider's immutable id of the account.
	Provider string
	Subject  string
	// Email and Name are the Profile's; Email is used only when TrustFailure is empty.
	Email string
	Name  string
	// TrustFailure is why the provider's proof of Email is not accepted ("" when it is).
	TrustFailure signin.Reason
	// Invitation is the pending Invitation the sign-in is to accept, nil for a plain sign-in.
	// It needs the provider's trusted email to be the invited address, and it opens a closed
	// sign-up for that address, as accepting an Invitation by password does (ADR 0042).
	Invitation *ent.Invitation
	// SurveyDue marks a User this sign-in creates as owing the Sign-up survey (the Instance asks
	// it). An Invitation sign-in never marks it, nor does a sign-in that creates nobody.
	SurveyDue bool

	// invited is set by SignInWithIdentity for an Invitation sign-in: the Invitation opens a
	// closed sign-up (ADR 0060), so neither the decision nor the transaction applies the rule.
	invited bool
}

// SigninRefusedError is a sign-in the decision refused; Reason is what the person is told.
type SigninRefusedError struct{ Reason signin.Reason }

func (e *SigninRefusedError) Error() string {
	return "accounts: provider sign-in refused: " + string(e.Reason)
}

// ProviderSignin is the User a provider sign-in is for, and what the sign-in did to the
// account.
type ProviderSignin struct {
	// User is the account as it stands after the sign-in (its session epoch included).
	User *ent.User
	// Created: the User was made by this sign-in.
	Created bool
	// LinkedIdentityID is the new Linked identity when the sign-in linked the provider
	// silently to an existing User holding the address (0 otherwise): the owner is to be
	// told, and may undo it with UnlinkIdentity.
	LinkedIdentityID int64
}

// SignInWithIdentity answers a provider callback: it reads the facts, takes the one
// decision (DecideSignin) and applies it. Only the User of the identity, or a new User, comes
// back; a refusal is a *SigninRefusedError. It does not start the session or write the login
// Audit entry: the caller does, once the Second factor is accounted for.
//
// An unknown subject whose trusted email a Verified User holds is linked to that User;
// when that User's address was never proven (an install with verification off) the link
// also clears the password and ends its sessions, so whoever proves the address is the
// one who controls it. An Unverified User's address is taken over: the provider's proof
// wins, the User becomes Verified, the pending password goes, a pending Invitation stays.
func (a *Accounts) SignInWithIdentity(ctx context.Context, in ProviderLogin) (*ProviderSignin, error) {
	if in.Invitation == nil {
		return a.signIn(ctx, in)
	}
	// An Invitation is for one address: a provider that vouches for another one creates and
	// links nothing, before the decision can. A provider whose proof of the email is not
	// accepted is left to the decision (a Linked identity may still sign in, then is held to
	// the address below).
	if in.TrustFailure == "" && !strings.EqualFold(strings.TrimSpace(in.Email), in.Invitation.Email) {
		return nil, &SigninRefusedError{Reason: signin.ReasonInvitationEmailMismatch}
	}
	in.invited = true
	in.SurveyDue = false
	signedIn, err := a.signIn(ctx, in)
	if err != nil {
		return nil, err
	}
	if !strings.EqualFold(signedIn.User.Email, in.Invitation.Email) {
		// A Linked identity of an account with another address: nothing was created.
		return nil, &SigninRefusedError{Reason: signin.ReasonInvitationEmailMismatch}
	}
	// The provider's proof of the address is the session proof AcceptInvitation asks of a
	// Verified User.
	if err := a.AcceptInvitation(ctx, in.Invitation, "", "", signedIn.User.Email); err != nil {
		return nil, err
	}
	return signedIn, nil
}

// signIn is SignInWithIdentity without an Invitation.
func (a *Accounts) signIn(ctx context.Context, in ProviderLogin) (*ProviderSignin, error) {
	for attempt := 0; ; attempt++ {
		known, holder, err := a.signinFacts(ctx, in)
		if err != nil {
			return nil, err
		}
		open := in.invited
		if !open {
			if open, err = a.SignupOpen(ctx); err != nil {
				return nil, err
			}
		}
		facts := SigninFacts{KnownIdentity: known != nil, TrustFailure: in.TrustFailure, SignupOpen: open}
		if holder != nil {
			facts.EmailHolder = &EmailHolder{Verified: holder.EmailVerifiedAt != nil, Proven: holder.EmailProvenAt != nil}
		}
		decision := DecideSignin(facts)
		switch decision.Outcome {
		case SigninKnown:
			return &ProviderSignin{User: known}, nil
		case SigninCreate:
			created, err := a.createFromProvider(ctx, in)
			if db.IsUniqueViolation(err) && attempt == 0 {
				// Two sign-ins of one person raced: read the facts again, where the winner's
				// User and identity are now visible.
				continue
			}
			if errors.Is(err, ErrSignupClosed) {
				// Another registration took the first place between the facts and the write.
				return nil, &SigninRefusedError{Reason: signin.ReasonSignupClosed}
			}
			if err != nil {
				return nil, err
			}
			return &ProviderSignin{User: created, Created: true}, nil
		case SigninLink:
			linked, identityID, err := a.linkToHolder(ctx, holder.ID, in)
			if db.IsUniqueViolation(err) && attempt == 0 {
				continue // the same sign-in raced itself: read the facts again
			}
			if err != nil {
				return nil, err
			}
			return &ProviderSignin{User: linked, LinkedIdentityID: identityID}, nil
		case SigninTakeOver:
			taken, err := a.takeOver(ctx, holder.ID, in)
			if (errors.Is(err, ErrAlreadyVerified) || db.IsUniqueViolation(err)) && attempt == 0 {
				// The registrant verified the address first (it is a Verified User now), or
				// the sign-in raced itself: read the facts again.
				continue
			}
			if err != nil {
				return nil, err
			}
			return &ProviderSignin{User: taken}, nil
		default:
			return nil, &SigninRefusedError{Reason: decision.Reason}
		}
	}
}

// signinFacts loads the User whose Linked identity is the provider's subject, and the User
// that holds the provider's email (looked up case-insensitively: the unique index is
// case-sensitive, so a different spelling of a held address must not create a duplicate).
// The email is read only when the provider's proof of it is accepted.
func (a *Accounts) signinFacts(ctx context.Context, in ProviderLogin) (known, holder *ent.User, err error) {
	identity, err := a.ent.UserIdentity.Query().
		Where(useridentity.Provider(in.Provider), useridentity.Subject(in.Subject)).
		WithUser().
		Only(ctx)
	switch {
	case err == nil:
		return identity.Edges.User, nil, nil
	case !ent.IsNotFound(err):
		return nil, nil, err
	}
	if in.TrustFailure != "" || in.Email == "" {
		return nil, nil, nil
	}
	holder, err = a.ent.User.Query().Where(user.EmailEqualFold(in.Email)).First(ctx)
	if ent.IsNotFound(err) {
		return nil, nil, nil
	}
	return nil, holder, err
}

// createFromProvider creates a Verified User for a trusted provider email, with the Linked
// identity, its first Workspace and `user.email_verify`, in one transaction, as a
// registration does when its link is followed (ADR 0030). The User has no password: the
// provider proved the address, so no mail is sent. Its email is proven now.
func (a *Accounts) createFromProvider(ctx context.Context, in ProviderLogin) (*ent.User, error) {
	email := strings.TrimSpace(in.Email)
	name := strings.TrimSpace(in.Name)
	if name == "" {
		name, _, _ = strings.Cut(email, "@")
	}
	u, _, err := a.verified(ctx, &proof{SignupPermits: true}, func(tx *ent.Client) (*ent.User, error) {
		if !in.invited {
			if err := a.permitSignup(ctx, tx); err != nil {
				return nil, err
			}
		}
		now := time.Now()
		u, err := tx.User.Create().
			SetName(name).
			SetEmail(email).
			SetEmailVerifiedAt(now).
			SetEmailProvenAt(now).
			SetNillableSignupSurveyDueAt(surveyDueAt(in.SurveyDue)).
			Save(ctx)
		if err != nil {
			return nil, err
		}
		if err := tx.UserIdentity.Create().SetUserID(u.ID).SetProvider(in.Provider).SetSubject(in.Subject).Exec(ctx); err != nil {
			return nil, err
		}
		return u, nil
	})
	return u, err
}

var (
	// ErrIdentityInUse is a provider account that is already linked to another User.
	ErrIdentityInUse = errors.New("accounts: the provider account is linked to another user")
	// ErrIdentityNotFound is an identity that is not the User's.
	ErrIdentityNotFound = errors.New("accounts: no such linked identity")
	// ErrLastSignIn is an unlink that would leave the User with no way to sign in.
	ErrLastSignIn = errors.New("accounts: the identity is the user's last way to sign in")
)

// Identities lists the User's Linked identities, oldest first.
func (a *Accounts) Identities(ctx context.Context, userID int64) ([]*ent.UserIdentity, error) {
	return a.ent.UserIdentity.Query().
		Where(useridentity.UserID(userID)).
		Order(ent.Asc(useridentity.FieldCreatedAt), ent.Asc(useridentity.FieldID)).
		All(ctx)
}

// LinkIdentity attaches a provider account to the User, whatever email the provider
// reports (the subject is the key, ADR 0048), and bumps the session epoch: the caller
// reissues the acting session from the returned User. A subject that already belongs to
// another User is ErrIdentityInUse; one that already belongs to this User changes nothing
// (added is false).
func (a *Accounts) LinkIdentity(ctx context.Context, userID int64, provider, subject string) (*ent.User, bool, error) {
	var saved *ent.User
	added := false
	err := a.bus.WithinTx(ctx, func(tx *ent.Client, pub events.Publisher) error {
		u, err := tx.User.Query().Where(user.ID(userID)).ForUpdate().Only(ctx)
		if err != nil {
			return err
		}
		holder, err := tx.UserIdentity.Query().
			Where(useridentity.Provider(provider), useridentity.Subject(subject)).
			Only(ctx)
		switch {
		case err == nil && holder.UserID == userID:
			saved = u
			return nil
		case err == nil:
			return ErrIdentityInUse
		case !ent.IsNotFound(err):
			return err
		}
		if err := tx.UserIdentity.Create().SetUserID(userID).SetProvider(provider).SetSubject(subject).Exec(ctx); err != nil {
			return err
		}
		saved, err = tx.User.UpdateOneID(userID).AddSessionEpoch(1).Save(ctx)
		if err != nil {
			return err
		}
		added = true
		return RecordUserAction(ctx, tx, pub, saved, events.ActionUserIdentityLink, map[string]any{"provider": provider})
	})
	if db.IsUniqueViolation(err) {
		return nil, false, ErrIdentityInUse
	}
	return saved, added, err
}

// UnlinkIdentity removes one of the User's Linked identities and bumps the session epoch.
// It is ErrIdentityNotFound for an identity that is not the User's and ErrLastSignIn when
// the User would be left with no password and no other identity. The User row is locked, so
// of two unlinks racing for the last two identities one is refused.
func (a *Accounts) UnlinkIdentity(ctx context.Context, userID, identityID int64) (*ent.User, error) {
	var saved *ent.User
	err := a.bus.WithinTx(ctx, func(tx *ent.Client, pub events.Publisher) error {
		u, err := tx.User.Query().Where(user.ID(userID)).ForUpdate().Only(ctx)
		if err != nil {
			return err
		}
		own, err := tx.UserIdentity.Query().Where(useridentity.UserID(userID)).All(ctx)
		if err != nil {
			return err
		}
		i := slices.IndexFunc(own, func(id *ent.UserIdentity) bool { return id.ID == identityID })
		if i < 0 {
			return ErrIdentityNotFound
		}
		if u.PasswordHash == "" && len(own) == 1 {
			return ErrLastSignIn
		}
		if err := tx.UserIdentity.DeleteOneID(identityID).Exec(ctx); err != nil {
			return err
		}
		saved, err = tx.User.UpdateOneID(userID).AddSessionEpoch(1).Save(ctx)
		if err != nil {
			return err
		}
		return RecordUserAction(ctx, tx, pub, saved, events.ActionUserIdentityUnlink, map[string]any{"provider": own[i].Provider})
	})
	return saved, err
}

// linkToHolder attaches the identity to the Verified User holding the provider's email. The
// provider's proof makes the address proven; every link ends the User's other sessions, and
// a User whose address was not proven before also loses its password, in the same
// transaction. It returns the User as it
// stands afterwards and the new identity's id.
func (a *Accounts) linkToHolder(ctx context.Context, holderID int64, in ProviderLogin) (*ent.User, int64, error) {
	var linked *ent.User
	var identityID int64
	err := a.bus.WithinTx(ctx, func(tx *ent.Client, pub events.Publisher) error {
		// Every link ends the other sessions (ADR 0048); the caller starts the new one from
		// the User returned here, so it holds the new epoch.
		// The provider's proof stamps the address only from empty, and the rows-changed count
		// says whether it did: that, and only that, is a domain-join step (ADR 0051).
		stamped, err := tx.User.Update().Where(user.ID(holderID), user.EmailProvenAtIsNil()).
			SetEmailProvenAt(time.Now()).ClearPasswordHash().Save(ctx)
		if err != nil {
			return err
		}
		if linked, err = tx.User.UpdateOneID(holderID).AddSessionEpoch(1).Save(ctx); err != nil {
			return err
		}
		if stamped > 0 {
			if _, err := applyDomainJoin(ctx, tx, pub, linked, proof{SignupPermits: true}); err != nil {
				return err
			}
		}
		identity, err := tx.UserIdentity.Create().SetUserID(holderID).SetProvider(in.Provider).SetSubject(in.Subject).Save(ctx)
		if err != nil {
			return err
		}
		identityID = identity.ID
		return RecordUserAction(ctx, tx, pub, linked, events.ActionUserIdentityLink, map[string]any{"provider": in.Provider})
	})
	return linked, identityID, err
}

// takeOver is the provider's proof winning over a registration nobody proved: the Unverified
// User becomes Verified with the provider's name, its pending password is cleared, earlier
// sessions end, and it gets the identity and its first Workspace, as a verify link would
// give it. A pending Invitation to the address is not touched. ErrAlreadyVerified when the
// User is no longer Unverified.
func (a *Accounts) takeOver(ctx context.Context, holderID int64, in ProviderLogin) (*ent.User, error) {
	name := strings.TrimSpace(in.Name)
	u, _, err := a.verified(ctx, &proof{SignupPermits: true}, func(tx *ent.Client) (*ent.User, error) {
		now := time.Now()
		upd := tx.User.Update().
			Where(user.ID(holderID), user.EmailVerifiedAtIsNil()).
			SetEmailVerifiedAt(now).
			SetEmailProvenAt(now).
			ClearPasswordHash().
			AddSessionEpoch(1)
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
		if err := tx.UserIdentity.Create().SetUserID(holderID).SetProvider(in.Provider).SetSubject(in.Subject).Exec(ctx); err != nil {
			return nil, err
		}
		return tx.User.Get(ctx, holderID)
	})
	return u, err
}

// UndoLink removes one Linked identity of a User and ends every session of the User: the
// "this was not me" answer to a silent link (ADR 0048). Unlike UnlinkIdentity it is not
// refused when the identity is the User's last way in (a never-proven address lost its
// password at the link; the reset flow gives it back). It reports false, and changes
// nothing, when the identity is already gone (a link followed twice).
func (a *Accounts) UndoLink(ctx context.Context, userID, identityID int64) (bool, error) {
	removed := false
	err := a.bus.WithinTx(ctx, func(tx *ent.Client, pub events.Publisher) error {
		identity, err := tx.UserIdentity.Query().Where(useridentity.ID(identityID), useridentity.UserID(userID)).Only(ctx)
		if ent.IsNotFound(err) {
			return nil
		}
		if err != nil {
			return err
		}
		if err := tx.UserIdentity.DeleteOneID(identity.ID).Exec(ctx); err != nil {
			return err
		}
		removed = true
		saved, err := tx.User.UpdateOneID(userID).AddSessionEpoch(1).Save(ctx)
		if err != nil {
			return err
		}
		return RecordUserAction(ctx, tx, pub, saved, events.ActionUserIdentityUnlink, map[string]any{"provider": identity.Provider})
	})
	return removed, err
}
