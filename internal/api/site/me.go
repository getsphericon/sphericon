package site

import (
	"context"

	"github.com/getsphericon/sphericon/ent"
	siteapi "github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/accounts"
	"github.com/getsphericon/sphericon/internal/api/auth"
	"github.com/getsphericon/sphericon/internal/consentrecords"
	"github.com/getsphericon/sphericon/internal/convert"
	"github.com/getsphericon/sphericon/internal/login"
	"github.com/getsphericon/sphericon/internal/ratelimit"
)

// SiteUserGetMe returns the authenticated user's profile. Auth is enforced by
// the security handler, so SiteAuth is always present here.
func (h *Handlers) SiteUserGetMe(ctx context.Context) (*siteapi.SiteUserResource, error) {
	a := auth.GetSiteAuth(ctx)
	if a == nil {
		return nil, auth.ErrUnauthorized
	}
	u, err := h.accounts.User(ctx, a.UserID)
	if err != nil {
		return nil, err
	}
	return h.userResource(ctx, u)
}

// userResource is the User's profile with the Sign-up survey due only while the User carries the
// marker and the Instance still asks the survey (a switch turned off or a lapsed license stops
// asking at once, for a User marked earlier too), and the required acceptances pending for them
// (ADR 0062).
func (h *Handlers) userResource(ctx context.Context, u *ent.User) (*siteapi.SiteUserResource, error) {
	pending, err := h.consentRecords.Pending(ctx, u.ID)
	if err != nil {
		return nil, err
	}
	res := mapper.UserToResource(u)
	res.SignupSurveyDue = u.SignupSurveyDueAt != nil && h.signupSurvey.Enabled()
	res.AcceptancesPending = mapper.SignupItemsToSite(pending)
	return res, nil
}

// SiteUserAcceptPending accepts every required acceptance pending for the signed-in User at the
// versions the screen showed (ADR 0062), and is their profile after it.
func (h *Handlers) SiteUserAcceptPending(ctx context.Context, req *siteapi.SiteAcceptPendingInput) (siteapi.SiteUserAcceptPendingRes, error) {
	a := auth.GetSiteAuth(ctx)
	if a == nil {
		return nil, auth.ErrUnauthorized
	}
	accepted := make([]consentrecords.ReAcceptance, len(req.Items))
	for i, item := range req.Items {
		accepted[i] = consentrecords.ReAcceptance{Key: string(item.Key), Version: string(item.Version)}
	}
	if err := h.consentRecords.AcceptPending(ctx, a.UserID, accepted); err != nil {
		return nil, err
	}
	u, err := h.accounts.User(ctx, a.UserID)
	if err != nil {
		return nil, err
	}
	return h.userResource(ctx, u)
}

// SiteUserUpdateMe changes the authenticated User's name, time zone and/or password through accounts,
// which holds the profile rules. The email is the login identity and is not editable here. A
// password change ended every session; the acting one continues under a fresh cookie.
func (h *Handlers) SiteUserUpdateMe(ctx context.Context, req *siteapi.SiteUpdateMeInput) (siteapi.SiteUserUpdateMeRes, error) {
	userID, err := sessionUserID(ctx)
	if err != nil {
		return nil, err
	}
	u, err := h.accounts.User(ctx, userID)
	if err != nil {
		return nil, err
	}
	change := accounts.ProfileChange{
		Timezone:        convert.StringPtr(req.Timezone),
		ClearTimezone:   req.Timezone.IsNull(),
		NewPassword:     req.NewPassword.Or(""),
		CurrentPassword: req.CurrentPassword.Or(""),
	}
	if name, ok := req.Name.Get(); ok {
		change.Name = &name
	}
	changed, err := h.accounts.ChangeProfile(ctx, u, change, h.proveCurrentPassword)
	if err != nil {
		return nil, err
	}
	profile, err := h.userResource(ctx, changed.User)
	if err != nil {
		return nil, err
	}
	res := &siteapi.SiteUserResourceHeaders{Response: *profile}
	if changed.PasswordChanged {
		cookie, err := h.sessions.Issue(changed.User, currentSession(ctx))
		if err != nil {
			return nil, err
		}
		res.SetCookie = siteapi.NewOptString(cookie.String())
	}
	return res, nil
}

// SiteUserEmailChange requests a change of the login email. It verifies the
// current password, and emails a confirmation link to the NEW address (or, when that
// address is Taken, a notice to its owner: the answer is the same) — the change only
// takes effect once that link is confirmed (SiteAuthConfirmEmailChange). Returns 202.
func (h *Handlers) SiteUserEmailChange(ctx context.Context, req *siteapi.SiteEmailChangeInput) (siteapi.SiteUserEmailChangeRes, error) {
	userID, err := sessionUserID(ctx)
	if err != nil {
		return nil, err
	}
	u, err := h.accounts.User(ctx, userID)
	if err != nil {
		return nil, err
	}

	newEmail, err := accounts.NewEmailFor(u, string(req.NewEmail))
	if err != nil {
		return nil, err
	}
	if err := h.proveCurrentPassword(ctx, u, req.CurrentPassword); err != nil {
		return nil, err
	}

	// Whether the new address is Taken is never told to the requester (ADR 0031): every
	// request takes a slot of the address's budget, mints a token and queues exactly one
	// mail job to that address. Free, it is the confirm link; Taken, it is a tokenless
	// notice to its owner. A job with no mail due (budget spent) is a no-send one.
	granted, err := h.attempts.Take(ctx, accounts.KindEmailChange, newEmail)
	if err != nil {
		return nil, err
	}
	taken, err := h.accounts.EmailTaken(ctx, newEmail)
	if err != nil {
		return nil, err
	}

	token, err := h.links.EmailChangeLink(u, newEmail)
	if err != nil {
		return nil, err
	}
	if taken {
		if err := h.sysmail.EnqueueAddressTakenNotice(ctx, newEmail, granted); err != nil {
			mailNotQueued(ctx, "AddressTakenNotice", err)
		}
	} else {
		if err := h.sysmail.EnqueueEmailChangeConfirm(ctx, newEmail, token, granted); err != nil {
			mailNotQueued(ctx, "EmailChangeConfirm", err)
		}
	}

	return &siteapi.SiteUserEmailChangeAccepted{}, nil
}

// SiteUserSignOutEverywhere ends every session of the authenticated user, on
// every device, and clears the acting browser's cookie: no session survives.
func (h *Handlers) SiteUserSignOutEverywhere(ctx context.Context) (*siteapi.SiteUserSignOutEverywhereNoContent, error) {
	a := auth.GetSiteAuth(ctx)
	if a == nil {
		return nil, auth.ErrUnauthorized
	}
	if err := h.accounts.EndSessions(ctx, a.UserID); err != nil {
		return nil, err
	}
	return &siteapi.SiteUserSignOutEverywhereNoContent{SetCookie: h.sessions.Cleared().String()}, nil
}

// proveCurrentPassword is the profile's proof of the current password (accounts.Prove): the
// login module's one prove path (throttle first, decoy hash, a failure counted on the Login
// budget). It returns nil when the password is the User's and login.ErrPasswordIncorrect when
// it is not; while the address's delay runs it returns the 429 error even for a right password.
func (h *Handlers) proveCurrentPassword(ctx context.Context, u *ent.User, password string) error {
	res, err := h.login.Prove(ctx, u.Email, u.PasswordHash, password)
	if err != nil {
		return err
	}
	if res.Proof == login.ProofThrottled {
		return loginDelayError(ctx, res.Delay)
	}
	return nil
}

// loginDelayError is the 429 rejection of a running Login throttle delay.
func loginDelayError(ctx context.Context, d login.Delay) error {
	return ratelimit.FromContext(ctx).Delay(ctx, ratelimit.PolicyLoginAccount, d.Limit, d.Wait, d.Now)
}
