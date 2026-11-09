package users

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/invitation"
	"github.com/getsphericon/sphericon/ent/pendingsignup"
	"github.com/getsphericon/sphericon/ent/predicate"
	"github.com/getsphericon/sphericon/internal/domain/identity/consentrecords"
	"github.com/getsphericon/sphericon/internal/domain/identity/invitations"
	"github.com/getsphericon/sphericon/internal/infra/db"
	"github.com/getsphericon/sphericon/internal/infra/events"
	"github.com/getsphericon/sphericon/internal/kernel/i18n"
	"github.com/getsphericon/sphericon/internal/kernel/validation"
)

// PendingSignupTTL is how long a Sign-in provider's sign-up waits for the Sign-up items to be
// answered (ADR 0062): as long as the flow that led to it.
const PendingSignupTTL = 10 * time.Minute

// ErrPendingSignupGone is a pending sign-up that can no longer be completed: expired, completed,
// declined, or overtaken (its address or provider account taken, its Invitation no longer pending).
var ErrPendingSignupGone = validation.UnknownReference("users: the pending sign-up is gone", i18n.ErrorsSigninPendingGone)

// HoldPending keeps what the provider proved as a pending sign-up instead of creating the User
// (ADR 0062), open until PendingSignupTTL after now. A pending sign-up of the same provider account
// is replaced, so only the latest flow's reference works; of two holds of the account that race,
// both answer with the one that was written.
func (a *Module) HoldPending(ctx context.Context, in ProviderLogin) (*ent.PendingSignup, error) {
	var held *ent.PendingSignup
	err := a.bus.WithinTx(ctx, func(tx *ent.Client, _ events.Publisher) error {
		if _, err := tx.PendingSignup.Delete().
			Where(pendingsignup.Provider(in.Provider), pendingsignup.Subject(in.Subject)).
			Exec(ctx); err != nil {
			return err
		}
		create := tx.PendingSignup.Create().
			SetProvider(in.Provider).
			SetSubject(in.Subject).
			SetEmail(strings.TrimSpace(in.Email)).
			SetName(strings.TrimSpace(in.Name)).
			SetExpiresAt(in.Now.Add(PendingSignupTTL)).
			SetCreatedAt(in.Now)
		if in.Invitation != nil {
			create = create.SetInvitationID(in.Invitation.ID)
		}
		err := create.OnConflictColumns(pendingsignup.FieldProvider, pendingsignup.FieldSubject).DoNothing().Exec(ctx)
		_, err = db.Inserted(err)
		if err != nil {
			return err
		}
		held, err = tx.PendingSignup.Query().
			Where(pendingsignup.Provider(in.Provider), pendingsignup.Subject(in.Subject)).
			Only(ctx)
		return err
	})
	return held, err
}

// openAt is the one definition of a pending sign-up still open at now: it has not expired.
func openAt(now time.Time) predicate.PendingSignup { return pendingsignup.ExpiresAtGT(now) }

// pendingSignup is the pending sign-up with id while it is still open at now, or
// ErrPendingSignupGone.
func (a *Module) pendingSignup(ctx context.Context, id int64, now time.Time) (*ent.PendingSignup, error) {
	p, err := a.ent.PendingSignup.Query().
		Where(pendingsignup.ID(id), openAt(now)).
		Only(ctx)
	if ent.IsNotFound(err) {
		return nil, ErrPendingSignupGone
	}
	return p, err
}

// CompletePendingSignupIn writes, in tx, the person's answers to the Sign-up items completing
// the pending sign-up with id (ADR 0062): it takes the pending sign-up (open at now), creates the
// Verified User with the provider's address and name, or takes over the Unverified User that
// holds the address, as a provider sign-in without items does, its Linked identity, the Consent
// records of the answers and the Membership of the Invitation the flow began from. The caller
// creates the first Workspace on the same transaction (internal/usecase/workspacecreation).
// surveyDue marks the User as owing the Sign-up survey, unless the sign-up accepts an Invitation.
// It returns the User and the pending sign-up it came from. ErrPendingSignupGone when the pending
// sign-up is no longer open, its Invitation no longer pending, or its address or provider
// account was taken meanwhile.
func (a *Module) CompletePendingSignupIn(ctx context.Context, tx *ent.Client, pub events.Publisher, id int64, consents consentrecords.Consents, surveyDue bool, now time.Time) (*ent.User, *ent.PendingSignup, error) {
	p, err := tx.PendingSignup.Query().
		Where(pendingsignup.ID(id), openAt(now)).
		ForUpdate().
		Only(ctx)
	if ent.IsNotFound(err) {
		return nil, nil, ErrPendingSignupGone
	}
	if err != nil {
		return nil, nil, err
	}
	if err := tx.PendingSignup.DeleteOneID(p.ID).Exec(ctx); err != nil {
		return nil, nil, err
	}
	var inv *ent.Invitation
	if p.InvitationID != nil {
		inv, err = tx.Invitation.Query().
			Where(invitation.ID(*p.InvitationID), invitations.PendingAt(now)).
			Only(ctx)
		if ent.IsNotFound(err) {
			return nil, nil, ErrPendingSignupGone
		}
		if err != nil {
			return nil, nil, err
		}
	}
	u, err := completeProviderSignup(ctx, tx, ProviderLogin{
		Provider: p.Provider, Subject: p.Subject, Email: p.Email, Name: p.Name,
		SurveyDue: surveyDue && inv == nil,
	}, now)
	if errors.Is(err, ErrEmailTaken) || errors.Is(err, ErrIdentityInUse) {
		// Another sign-up took the address or the provider account first.
		return nil, nil, ErrPendingSignupGone
	}
	if err != nil {
		return nil, nil, err
	}
	if err := consentrecords.Write(ctx, tx, u.ID, consents); err != nil {
		return nil, nil, err
	}
	if inv != nil {
		// The provider's proof of the address is the session proof the acceptance asks of a
		// Verified User, as on a provider sign-up without items.
		if err := a.acceptInvitationIn(ctx, tx, pub, inv, newAccount{}, u.Email); err != nil {
			return nil, nil, err
		}
	}
	return u, p, nil
}

// completeProviderSignup is the sign-in decision of a pending sign-up, taken again in tx on what
// it then finds, with the Sign-up items answered: the provider's address nobody holds creates the
// Verified User, one an Unverified User holds takes that User over. Anything else (the provider
// account linked meanwhile, the address verified by the person who signed up with it) is
// ErrPendingSignupGone.
func completeProviderSignup(ctx context.Context, tx *ent.Client, in ProviderLogin, now time.Time) (*ent.User, error) {
	known, holder, err := signinFacts(ctx, tx, in)
	if err != nil {
		return nil, err
	}
	switch DecideSignin(factsOf(known, holder, in)).Outcome {
	case SigninCreate:
		return CreateProviderUserIn(ctx, tx, in, now)
	case SigninTakeOver:
		u, err := TakeOverIn(ctx, tx, holder.ID, in, now)
		if errors.Is(err, ErrAlreadyVerified) {
			return nil, ErrPendingSignupGone
		}
		return u, err
	default:
		return nil, ErrPendingSignupGone
	}
}

// declinePendingSignup deletes the pending sign-up with id: the person declined the Sign-up items
// and nothing is created. A pending sign-up already gone changes nothing.
func (a *Module) declinePendingSignup(ctx context.Context, id int64) error {
	_, err := a.ent.PendingSignup.Delete().Where(pendingsignup.ID(id)).Exec(ctx)
	return err
}

// PruneExpiredPendingSignups deletes the pending sign-ups no longer open at now, so what an
// abandoned provider sign-up proved is not kept, and returns how many went.
func (a *Module) PruneExpiredPendingSignups(ctx context.Context, now time.Time) (int, error) {
	return a.ent.PendingSignup.Delete().Where(pendingsignup.ExpiresAtLTE(now)).Exec(ctx)
}
