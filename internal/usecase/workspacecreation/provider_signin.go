package workspacecreation

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/internal/domain/identity/consentrecords"
	"github.com/getsphericon/sphericon/internal/domain/identity/signin"
	"github.com/getsphericon/sphericon/internal/domain/identity/users"
	"github.com/getsphericon/sphericon/internal/infra/db"
	"github.com/getsphericon/sphericon/internal/infra/events"
)

// SignInWithIdentity answers a provider callback: it reads the facts, takes the one
// decision (users.DecideSignin) and applies it. Only the User of the identity, or a new User, comes
// back; a refusal is a *users.SigninRefusedError. It does not start the session or write the login
// Audit entry: the caller does, once the Second factor is accounted for.
//
// An unknown subject whose trusted email a Verified User holds is linked to that User;
// when that User's address was never proven (an install with verification off) the link
// also clears the password and ends its sessions, so whoever proves the address is the
// one who controls it. An Unverified User's address is taken over: the provider's proof
// wins, the User becomes Verified, the pending password goes, a pending Invitation stays.
func (u *UseCase) SignInWithIdentity(ctx context.Context, in users.ProviderLogin) (*users.ProviderSignin, error) {
	if in.Invitation == nil {
		return u.signIn(ctx, in)
	}
	// An Invitation is for one address: a provider that vouches for another one creates and
	// links nothing, before the decision can. A provider whose proof of the email is not
	// accepted is left to the decision (a Linked identity may still sign in, then is held to
	// the address below).
	if in.TrustFailure == "" && !strings.EqualFold(strings.TrimSpace(in.Email), in.Invitation.Email) {
		return nil, &users.SigninRefusedError{Reason: signin.ReasonInvitationEmailMismatch}
	}
	in.SurveyDue = false
	signedIn, err := u.signIn(ctx, in)
	if err != nil {
		return nil, err
	}
	if signedIn.Pending != nil {
		// The pending sign-up keeps the Invitation, which its completion accepts.
		return signedIn, nil
	}
	if !strings.EqualFold(signedIn.User.Email, in.Invitation.Email) {
		// A Linked identity of an account with another address: nothing was created.
		return nil, &users.SigninRefusedError{Reason: signin.ReasonInvitationEmailMismatch}
	}
	// The provider's proof of the address is the session proof AcceptInvitation asks of a
	// Verified User.
	if err := u.users.AcceptInvitation(ctx, in.Invitation, users.InvitationAcceptance{SessionEmail: signedIn.User.Email}); err != nil {
		return nil, err
	}
	return signedIn, nil
}

// signIn is SignInWithIdentity without an Invitation.
func (u *UseCase) signIn(ctx context.Context, in users.ProviderLogin) (*users.ProviderSignin, error) {
	for attempt := 0; ; attempt++ {
		decision, known, holder, err := u.users.DecideSigninFor(ctx, in)
		if err != nil {
			return nil, err
		}
		switch decision.Outcome {
		case users.SigninKnown:
			return &users.ProviderSignin{User: known}, nil
		case users.SigninCreate:
			created, err := u.createFromProvider(ctx, in)
			if db.IsUniqueViolation(err) && attempt == 0 {
				// Two sign-ins of one person raced: read the facts again, where the winner's
				// User and identity are now visible.
				continue
			}
			if err != nil {
				return nil, err
			}
			return &users.ProviderSignin{User: created, Created: true}, nil
		case users.SigninPending:
			held, err := u.users.HoldPending(ctx, in)
			if db.IsUniqueViolation(err) && attempt == 0 {
				continue // the same sign-in raced itself: read the facts again
			}
			if err != nil {
				return nil, err
			}
			return &users.ProviderSignin{Pending: held}, nil
		case users.SigninLink:
			linked, identityID, err := u.users.LinkToHolder(ctx, holder.ID, in)
			if db.IsUniqueViolation(err) && attempt == 0 {
				continue // the same sign-in raced itself: read the facts again
			}
			if err != nil {
				return nil, err
			}
			return &users.ProviderSignin{User: linked, LinkedIdentityID: identityID}, nil
		case users.SigninTakeOver:
			taken, err := u.takeOver(ctx, holder.ID, in)
			if (errors.Is(err, users.ErrAlreadyVerified) || db.IsUniqueViolation(err)) && attempt == 0 {
				// The person who signed up verified the address first (it is a Verified User now), or
				// the sign-in raced itself: read the facts again.
				continue
			}
			if err != nil {
				return nil, err
			}
			return &users.ProviderSignin{User: taken}, nil
		default:
			return nil, &users.SigninRefusedError{Reason: decision.Reason}
		}
	}
}

// createFromProvider creates a Verified User for a trusted provider email, with the Linked
// identity, its first Workspace and `user.email_verify`, in one transaction, as a
// Sign-up does when its link is followed (ADR 0030). The User has no password: the
// provider proved the address, so no mail is sent. Its email is proven now.
func (u *UseCase) createFromProvider(ctx context.Context, in users.ProviderLogin) (*ent.User, error) {
	created, _, err := u.verified(ctx, provenNow, func(tx *ent.Client, _ events.Publisher) (*ent.User, error) {
		return users.CreateProviderUserIn(ctx, tx, in)
	})
	return created, err
}

// takeOver is the provider's proof winning over a Sign-up nobody proved: the Unverified
// User becomes Verified with the provider's name, its pending password is cleared, earlier
// sessions end, and it gets the identity and its first Workspace, as a verify link would
// give it. A pending Invitation to the address is not touched. users.ErrAlreadyVerified when the
// User is no longer Unverified.
func (u *UseCase) takeOver(ctx context.Context, holderID int64, in users.ProviderLogin) (*ent.User, error) {
	taken, _, err := u.verified(ctx, provenNow, func(tx *ent.Client, _ events.Publisher) (*ent.User, error) {
		return users.TakeOverIn(ctx, tx, holderID, in)
	})
	return taken, err
}

// CompletePendingSignup is the person's answers to the Sign-up items completing the pending
// sign-up with id (ADR 0062): in one transaction it takes the pending sign-up (open at now), creates
// the Verified User with the provider's address and name, or takes over the Unverified User that
// holds the address, as a provider sign-in without items does, its Linked identity, the Consent records
// of the answers, the Membership of the Invitation the flow began from (users.Module.CompletePendingSignupIn),
// and the first Workspace, as a provider sign-up without items does. surveyDue marks the User as owing the
// Sign-up survey, unless the sign-up accepts an Invitation. It returns the User and the pending sign-up it
// came from. users.ErrPendingSignupGone when the pending sign-up is no longer open, its Invitation no longer
// pending, or its address or provider account was taken meanwhile; nothing is written then.
func (u *UseCase) CompletePendingSignup(ctx context.Context, id int64, consents consentrecords.Consents, surveyDue bool, now time.Time) (*ent.User, *ent.PendingSignup, error) {
	var taken *ent.PendingSignup
	completed, _, err := u.verified(ctx, provenNow, func(tx *ent.Client, pub events.Publisher) (*ent.User, error) {
		usr, p, err := u.users.CompletePendingSignupIn(ctx, tx, pub, id, consents, surveyDue, now)
		taken = p
		return usr, err
	})
	if db.IsUniqueViolation(err) {
		// Another sign-up took the address or the provider account first.
		return nil, nil, users.ErrPendingSignupGone
	}
	if err != nil {
		return nil, nil, err
	}
	return completed, taken, nil
}
