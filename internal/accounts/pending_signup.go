package accounts

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/invitation"
	"github.com/getsphericon/sphericon/ent/pendingsignup"
	"github.com/getsphericon/sphericon/ent/predicate"
	"github.com/getsphericon/sphericon/internal/consentrecords"
	"github.com/getsphericon/sphericon/internal/db"
	"github.com/getsphericon/sphericon/internal/events"
	"github.com/getsphericon/sphericon/internal/validation"
)

// PendingSignupTTL is how long a Sign-in provider's sign-up waits for the Sign-up items to be
// answered (ADR 0062): as long as the flow that led to it.
const PendingSignupTTL = 10 * time.Minute

// ErrPendingSignupGone is a pending sign-up that can no longer be completed: expired, completed,
// declined, or overtaken (its address or provider account taken, its Invitation no longer pending).
var ErrPendingSignupGone = validation.UnknownReference("accounts: the pending sign-up is gone", "errors.signin_pending_gone")

// holdPending keeps what the provider proved as a pending sign-up instead of creating the User
// (ADR 0062), open until PendingSignupTTL after now. A pending sign-up of the same provider account
// is replaced, so only the latest flow's reference works.
func (a *Accounts) holdPending(ctx context.Context, in ProviderLogin) (*ent.PendingSignup, error) {
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
		var err error
		held, err = create.Save(ctx)
		return err
	})
	return held, err
}

// openAt is the one definition of a pending sign-up still open at now: it has not expired.
func openAt(now time.Time) predicate.PendingSignup { return pendingsignup.ExpiresAtGT(now) }

// PendingSignup is the pending sign-up with id while it is still open at now, or
// ErrPendingSignupGone.
func (a *Accounts) PendingSignup(ctx context.Context, id int64, now time.Time) (*ent.PendingSignup, error) {
	p, err := a.ent.PendingSignup.Query().
		Where(pendingsignup.ID(id), openAt(now)).
		Only(ctx)
	if ent.IsNotFound(err) {
		return nil, ErrPendingSignupGone
	}
	return p, err
}

// CompletePendingSignup is the person's answers to the Sign-up items completing the pending
// sign-up with id (ADR 0062): in one transaction it takes the pending sign-up (open at now), creates
// the Verified User with the provider's address and name, or takes over the Unverified User that
// holds the address, as a provider sign-in without items does, its Linked identity, the Consent records
// of the answers, the Membership of the Invitation the flow began from, and the first Workspace, as
// a provider sign-up without items does. surveyDue marks the User as owing the Sign-up survey,
// unless the sign-up accepts an Invitation. It returns the User and the pending sign-up it came
// from. ErrPendingSignupGone when the pending sign-up is no longer open, its Invitation no longer
// pending, or its address or provider account was taken meanwhile; nothing is written then.
func (a *Accounts) CompletePendingSignup(ctx context.Context, id int64, consents consentrecords.Consents, surveyDue bool, now time.Time) (*ent.User, *ent.PendingSignup, error) {
	var taken *ent.PendingSignup
	u, _, err := a.verified(ctx, provenNow, func(tx *ent.Client, pub events.Publisher) (*ent.User, error) {
		p, err := tx.PendingSignup.Query().
			Where(pendingsignup.ID(id), openAt(now)).
			ForUpdate().
			Only(ctx)
		if ent.IsNotFound(err) {
			return nil, ErrPendingSignupGone
		}
		if err != nil {
			return nil, err
		}
		taken = p
		if err := tx.PendingSignup.DeleteOneID(p.ID).Exec(ctx); err != nil {
			return nil, err
		}
		var inv *ent.Invitation
		if p.InvitationID != nil {
			inv, err = tx.Invitation.Query().
				Where(invitation.ID(*p.InvitationID), pendingInvitation(now)).
				Only(ctx)
			if ent.IsNotFound(err) {
				return nil, ErrPendingSignupGone
			}
			if err != nil {
				return nil, err
			}
		}
		u, err := completeProviderSignup(ctx, tx, ProviderLogin{
			Provider: p.Provider, Subject: p.Subject, Email: p.Email, Name: p.Name,
			SurveyDue: surveyDue && inv == nil,
		})
		if err != nil {
			return nil, err
		}
		if err := consentrecords.Write(ctx, tx, u.ID, consents); err != nil {
			return nil, err
		}
		if inv != nil {
			// The provider's proof of the address is the session proof the acceptance asks of a
			// Verified User, as on a provider sign-up without items.
			if err := a.acceptInvitationIn(ctx, tx, pub, inv, newAccount{}, u.Email); err != nil {
				return nil, err
			}
		}
		return u, nil
	})
	if db.IsUniqueViolation(err) {
		// Another sign-up took the address or the provider account first.
		return nil, nil, ErrPendingSignupGone
	}
	if err != nil {
		return nil, nil, err
	}
	return u, taken, nil
}

// completeProviderSignup is the sign-in decision of a pending sign-up, taken again in tx on what
// it then finds, with the Sign-up items answered: the provider's address nobody holds creates the
// Verified User, one an Unverified User holds takes that User over. Anything else (the provider
// account linked meanwhile, the address verified by the person who signed up with it) is
// ErrPendingSignupGone.
func completeProviderSignup(ctx context.Context, tx *ent.Client, in ProviderLogin) (*ent.User, error) {
	known, holder, err := signinFacts(ctx, tx, in)
	if err != nil {
		return nil, err
	}
	switch DecideSignin(factsOf(known, holder, in)).Outcome {
	case SigninCreate:
		return createProviderUser(ctx, tx, in)
	case SigninTakeOver:
		u, err := takeOverIn(ctx, tx, holder.ID, in)
		if errors.Is(err, ErrAlreadyVerified) {
			return nil, ErrPendingSignupGone
		}
		return u, err
	default:
		return nil, ErrPendingSignupGone
	}
}

// DeclinePendingSignup deletes the pending sign-up with id: the person declined the Sign-up items
// and nothing is created. A pending sign-up already gone changes nothing.
func (a *Accounts) DeclinePendingSignup(ctx context.Context, id int64) error {
	_, err := a.ent.PendingSignup.Delete().Where(pendingsignup.ID(id)).Exec(ctx)
	return err
}

// PruneExpiredPendingSignups deletes the pending sign-ups no longer open at now, so what an
// abandoned provider sign-up proved is not kept, and returns how many went.
func (a *Accounts) PruneExpiredPendingSignups(ctx context.Context, now time.Time) (int, error) {
	return a.ent.PendingSignup.Delete().Where(pendingsignup.ExpiresAtLTE(now)).Exec(ctx)
}
