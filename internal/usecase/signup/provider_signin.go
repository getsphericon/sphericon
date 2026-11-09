package signup

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"time"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/consentrecord"
	"github.com/getsphericon/sphericon/internal/domain/identity/consentrecords"
	"github.com/getsphericon/sphericon/internal/domain/identity/invitations"
	"github.com/getsphericon/sphericon/internal/domain/identity/secondfactors"
	"github.com/getsphericon/sphericon/internal/domain/identity/signinprovider"
	"github.com/getsphericon/sphericon/internal/domain/identity/signupitems"
	"github.com/getsphericon/sphericon/internal/domain/identity/users"
	"github.com/getsphericon/sphericon/internal/infra/events"
)

// SignInConfig is what a sign-in through a Sign-in provider needs besides the users module.
type SignInConfig struct {
	// Providers completes a flow with the provider and judges its proof of the email.
	Providers *signinprovider.Providers
	// Invitations finds the pending Invitation a flow began from.
	Invitations *invitations.Module
	// SignupItems says whether the Instance asks Sign-up items (ADR 0062).
	SignupItems *signupitems.Module
	// SecondFactors says whether the User signing in has a Second factor due (ADR 0020).
	SecondFactors *secondfactors.Module
}

// ProviderSignInInput is a Sign-in provider's answer to a flow this browser began: the flow its
// driver opened and checked, the code the provider sent back and, for a link flow, the session
// the browser holds.
type ProviderSignInInput struct {
	Flow signinprovider.Flow
	Code string
	// Session is the User and the session epoch of the session the browser presented; zero when
	// it presented none that verifies. Only a link flow reads it.
	Session SessionHolder
}

// SessionHolder is who a session was issued to and under which session epoch.
type SessionHolder struct {
	UserID int64
	Epoch  int64
}

// ProviderSignInOutcome is what a sign-in through a Sign-in provider did.
type ProviderSignInOutcome int

const (
	// SignedIn: the User is signed in; `user.login` is recorded and a session is due.
	SignedIn ProviderSignInOutcome = iota + 1
	// SecondFactorDue: the User has a Second factor, which the second login step asks for before
	// any session (ADR 0020).
	SecondFactorDue
	// PendingSignupKept: a Pending sign-up was kept for the Sign-up items to be answered (ADR 0062).
	PendingSignupKept
	// IdentityLinked: a link flow added the provider's subject to the User it names as a Linked
	// identity (or found it there); that User's session is reissued.
	IdentityLinked
)

// ProviderSignInResult is what happened, with what the driver needs to answer it.
type ProviderSignInResult struct {
	Outcome ProviderSignInOutcome
	// User is the User signed in, due a Second factor or linked; nil for a Pending sign-up.
	User *ent.User
	// Pending is the Pending sign-up of PendingSignupKept.
	Pending *ent.PendingSignup
	// ChallengeBinding is what the login challenge of SecondFactorDue is keyed to
	// (secondfactors.Module.ChallengeBinding).
	ChallengeBinding string
	// Added: the link flow added the Linked identity now; false when the User already had it,
	// which makes the link a fresh provider login.
	Added bool
	// At is the time of the provider login, by the Instance clock.
	At time.Time
}

// SignInWithProvider is the sign-in through a Sign-in provider (ADR 0048, ADR 0111): it completes
// the flow with the provider's code and applies the answer. A link flow adds the provider's subject
// to the User whose live session the browser holds. Any other flow is a sign-in: the Invitation it
// began from, the Sign-up items in effect and the provider's proof of the email decide on the User
// (signInWithIdentity), and a User with a Second factor is due it; a User signed in has
// `user.login` recorded, whose failure only logs, since the sign-in already holds.
//
// A refusal is a *users.SigninRefusedError naming the Reason the person is shown: the provider's
// answer is not usable, the Invitation is no longer pending, the link flow's session is not live,
// or the decision refused.
func (u *UseCase) SignInWithProvider(ctx context.Context, in ProviderSignInInput) (*ProviderSignInResult, error) {
	cfg := u.providers
	flow := in.Flow
	profile, err := cfg.Providers.Complete(ctx, flow, in.Code)
	if err != nil {
		// The error names the provider's failure, never a token or the subject.
		slog.WarnContext(ctx, "signin: the provider's answer was not usable", "provider", flow.Provider, "error", err)
		return nil, &users.SigninRefusedError{Reason: signinprovider.ReasonProviderError}
	}
	at := u.now()
	if flow.LinkUserID != 0 {
		return u.linkIdentity(ctx, flow, in.Session, profile.Subject, at)
	}

	var invite *ent.Invitation
	if flow.Invitation != "" {
		invite, err = cfg.Invitations.PendingInvitation(ctx, flow.Invitation)
		if errors.Is(err, invitations.ErrInvitationLinkNotFound) {
			return nil, &users.SigninRefusedError{Reason: signinprovider.ReasonInvitationInvalid}
		}
		if err != nil {
			return nil, err
		}
	}
	asked, err := cfg.SignupItems.Asked(ctx)
	if err != nil {
		return nil, err
	}
	signedIn, err := u.signInWithIdentity(ctx, users.ProviderLogin{
		Provider:            flow.Provider,
		Subject:             profile.Subject,
		Email:               profile.Email,
		Name:                profile.Name,
		TrustFailure:        cfg.Providers.Trust(flow.Provider, profile),
		Invitation:          invite,
		SurveyDue:           u.signUp.SurveyDue != nil && u.signUp.SurveyDue(),
		SignupItemsInEffect: asked,
		Now:                 at,
	})
	if err != nil {
		return nil, err
	}
	if signedIn.Pending != nil {
		return &ProviderSignInResult{Outcome: PendingSignupKept, Pending: signedIn.Pending, At: at}, nil
	}
	usr := signedIn.User
	due, err := cfg.SecondFactors.Active(ctx, usr.ID)
	if err != nil {
		return nil, err
	}
	if due {
		// A provider replaces only the password step: an enrolled Second factor still gets the
		// challenge of ADR 0020, finished through the same second step (and the same Login
		// throttle) as after a password.
		binding, err := cfg.SecondFactors.ChallengeBinding(ctx, usr)
		if err != nil {
			return nil, err
		}
		return &ProviderSignInResult{Outcome: SecondFactorDue, User: usr, ChallengeBinding: binding, At: at}, nil
	}
	if err := u.users.RecordLogin(ctx, usr, flow.Provider); err != nil {
		slog.ErrorContext(ctx, "audit: login not recorded", "error", err)
	}
	return &ProviderSignInResult{Outcome: SignedIn, User: usr, At: at}, nil
}

// linkIdentity finishes a link flow: only the browser whose live session is the User's the flow
// names adds the provider's subject to that User (ADR 0048). A session of another User, or one a
// later session epoch ended, is ReasonInvalidState; a subject of another User ReasonIdentityInUse.
func (u *UseCase) linkIdentity(ctx context.Context, flow signinprovider.Flow, session SessionHolder, subject string, at time.Time) (*ProviderSignInResult, error) {
	if session.UserID != flow.LinkUserID {
		return nil, &users.SigninRefusedError{Reason: signinprovider.ReasonInvalidState}
	}
	holder, err := u.users.User(ctx, session.UserID)
	if err != nil {
		return nil, err
	}
	if holder.SessionEpoch != session.Epoch {
		return nil, &users.SigninRefusedError{Reason: signinprovider.ReasonInvalidState}
	}
	linked, added, err := u.users.LinkIdentity(ctx, flow.LinkUserID, flow.Provider, subject)
	if errors.Is(err, users.ErrIdentityInUse) {
		return nil, &users.SigninRefusedError{Reason: signinprovider.ReasonIdentityInUse}
	}
	if err != nil {
		return nil, err
	}
	return &ProviderSignInResult{Outcome: IdentityLinked, User: linked, Added: added, At: at}, nil
}

// signInWithIdentity applies a provider's answer to a sign-in: it reads the facts, takes the one
// decision (users.DecideSignin) and applies it. Only the User of the identity, or a new User, comes
// back; a refusal is a *users.SigninRefusedError. It writes no login Audit entry:
// SignInWithProvider does, once the Second factor is accounted for.
//
// An unknown subject whose trusted email a Verified User holds is linked to that User;
// when that User's address was never proven (an install with verification off) the link
// also clears the password and ends its sessions, so whoever proves the address is the
// one who controls it. An Unverified User's address is taken over: the provider's proof
// wins, the User becomes Verified, the pending password goes, a pending Invitation stays.
func (u *UseCase) signInWithIdentity(ctx context.Context, in users.ProviderLogin) (*users.ProviderSignin, error) {
	if in.Invitation == nil {
		return u.signIn(ctx, in)
	}
	// An Invitation is for one address: a provider that vouches for another one creates and
	// links nothing, before the decision can. A provider whose proof of the email is not
	// accepted is left to the decision (a Linked identity may still sign in, then is held to
	// the address below).
	if in.TrustFailure == "" && !strings.EqualFold(strings.TrimSpace(in.Email), in.Invitation.Email) {
		return nil, &users.SigninRefusedError{Reason: signinprovider.ReasonInvitationEmailMismatch}
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
		return nil, &users.SigninRefusedError{Reason: signinprovider.ReasonInvitationEmailMismatch}
	}
	// The provider's proof of the address is the session proof AcceptInvitation asks of a
	// Verified User.
	if err := u.users.AcceptInvitation(ctx, in.Invitation, users.InvitationAcceptance{SessionEmail: signedIn.User.Email}); err != nil {
		return nil, err
	}
	return signedIn, nil
}

// signIn is signInWithIdentity without an Invitation.
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
			if lostRace(err) && attempt == 0 {
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
			if err != nil {
				return nil, err
			}
			return &users.ProviderSignin{Pending: held}, nil
		case users.SigninLink:
			linked, identityID, err := u.users.LinkToHolder(ctx, holder.ID, in)
			if lostRace(err) && attempt == 0 {
				continue // the same sign-in raced itself: read the facts again
			}
			if err != nil {
				return nil, err
			}
			return &users.ProviderSignin{User: linked, LinkedIdentityID: identityID}, nil
		case users.SigninTakeOver:
			taken, err := u.takeOver(ctx, holder.ID, in)
			if (errors.Is(err, users.ErrAlreadyVerified) || lostRace(err)) && attempt == 0 {
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

// lostRace reports whether err is a write of a provider sign-in that another sign-in of the same
// person won: the address or the provider account is taken now.
func lostRace(err error) bool {
	return errors.Is(err, users.ErrEmailTaken) || errors.Is(err, users.ErrIdentityInUse)
}

// createFromProvider creates a Verified User for a trusted provider email, with the Linked
// identity, its first Workspace and `user.email_verify`, in one transaction, as a
// Sign-up does when its link is followed (ADR 0030). The User has no password: the
// provider proved the address, so no mail is sent. Its email is proven now.
func (u *UseCase) createFromProvider(ctx context.Context, in users.ProviderLogin) (*ent.User, error) {
	created, _, err := u.verified(ctx, provenNow, func(tx *ent.Client, _ events.Publisher) (*ent.User, error) {
		return users.CreateProviderUserIn(ctx, tx, in, u.now())
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
		return users.TakeOverIn(ctx, tx, holderID, in, u.now())
	})
	return taken, err
}

// CompletePendingSignup is the person's answers to the Sign-up items completing the pending
// sign-up a signed reference names (ADR 0062, users.Links.PendingSignup). A reference that names
// nothing open is users.ErrPendingSignupGone. The answers are checked next, as the provider screen's
// (SignUpConfig.Consents), and a refused one is the *validation.FieldsError that names it, with
// nothing written and the pending sign-up still open. Then, in one transaction, it takes the
// pending sign-up (open at now), creates the Verified User with the provider's address and name, or
// takes over the Unverified User that holds the address, as a provider sign-in without items does,
// its Linked identity, the Consent records of the answers, the Membership of the Invitation the
// flow began from (users.Module.CompletePendingSignupIn), and the first Workspace, as a provider
// sign-up without items does. The User owes the Sign-up survey when the Instance asks for it
// (SignUpConfig.SurveyDue), unless the sign-up accepts an Invitation. The sign-in it ends is
// recorded as `user.login` by the provider, as SignInWithProvider records one; an entry that fails
// is logged and does not undo the sign-up. It returns the User and the pending sign-up it came
// from. users.ErrPendingSignupGone when the pending sign-up is no longer open, its Invitation no
// longer pending, or its address or provider account was taken meanwhile; nothing is written then.
func (u *UseCase) CompletePendingSignup(ctx context.Context, reference string, answers []consentrecords.Answer) (*SignupCompleted, error) {
	pending, err := u.links.PendingSignup(ctx, reference)
	if err != nil {
		return nil, err
	}
	id := pending.ID
	consents, err := u.signUp.Consents.Check(ctx, answers, consentrecord.SourceProviderScreen)
	if err != nil {
		return nil, err
	}
	surveyDue := u.signUp.SurveyDue != nil && u.signUp.SurveyDue()
	now := u.now()
	var taken *ent.PendingSignup
	completed, _, err := u.verified(ctx, provenNow, func(tx *ent.Client, pub events.Publisher) (*ent.User, error) {
		usr, p, err := u.users.CompletePendingSignupIn(ctx, tx, pub, id, consents, surveyDue, now)
		taken = p
		return usr, err
	})
	if err != nil {
		return nil, err
	}
	if err := u.users.RecordLogin(ctx, completed, taken.Provider); err != nil {
		slog.ErrorContext(ctx, "audit: login not recorded", "error", err)
	}
	return &SignupCompleted{User: completed, PendingSignup: taken}, nil
}

// SignupCompleted is a completed pending sign-up: the User it made Verified and the pending
// sign-up it came from.
type SignupCompleted struct {
	User          *ent.User
	PendingSignup *ent.PendingSignup
}
