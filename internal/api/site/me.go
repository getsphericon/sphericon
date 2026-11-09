package site

import (
	"context"

	"github.com/getsphericon/sphericon/ent"
	siteapi "github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/api/auth"
	"github.com/getsphericon/sphericon/internal/domain/identity/signupitems"
	"github.com/getsphericon/sphericon/internal/domain/identity/users"
	"github.com/getsphericon/sphericon/internal/kernel/convert"
)

// SiteUserGetMe returns the authenticated user's profile. Auth is enforced by
// the security handler, so SiteAuth is always present here.
func (h *Handlers) SiteUserGetMe(ctx context.Context) (*siteapi.SiteUserResource, error) {
	u, err := sessionUser(ctx)
	if err != nil {
		return nil, err
	}
	pending, err := h.consentRecords.Pending(ctx, u.ID)
	if err != nil {
		return nil, err
	}
	return h.userResource(u, pending), nil
}

// userResource is the User's profile with the Sign-up survey due only while the User carries the
// marker and the Instance still asks the survey (a switch turned off or a lapsed license stops
// asking at once, for a User marked earlier too), and the required acceptances pending for them
// (ADR 0062).
func (h *Handlers) userResource(u *ent.User, pending []signupitems.Document) *siteapi.SiteUserResource {
	res := mapper.UserToResource(u)
	res.SignupSurveyDue = u.SignupSurveyDueAt != nil && h.signupSurvey.Enabled()
	res.AcceptancesPending = mapper.SignupItemsToSite(pending)
	return res
}

// SiteUserAcceptPending accepts every required acceptance pending for the signed-in User at the
// versions the screen showed (ADR 0062), and is their profile after it.
func (h *Handlers) SiteUserAcceptPending(ctx context.Context, req *siteapi.SiteAcceptPendingInput) (*siteapi.SiteUserResource, error) {
	u, err := sessionUser(ctx)
	if err != nil {
		return nil, err
	}
	pending, err := h.consentRecords.AcceptPending(ctx, u.ID, inputs.ReAcceptances(req.Items))
	if err != nil {
		return nil, err
	}
	return h.userResource(u, pending), nil
}

// SiteUserUpdateMe changes the authenticated User's name, time zone, Regional format and/or password through users,
// which holds the profile rules. The email is the login identity and is not editable here. A
// password change ended every session; the acting one continues under a fresh cookie.
func (h *Handlers) SiteUserUpdateMe(ctx context.Context, req *siteapi.SiteUpdateMeInput) (*siteapi.SiteUserResourceHeaders, error) {
	u, err := sessionUser(ctx)
	if err != nil {
		return nil, err
	}
	change := users.ProfileChange{
		Name:                convert.StringPtr(req.Name),
		Timezone:            convert.StringPtr(req.Timezone),
		ClearTimezone:       req.Timezone.IsNull(),
		RegionalFormat:      convert.StringPtr(req.RegionalFormat),
		ClearRegionalFormat: req.RegionalFormat.IsNull(),
		NewPassword:         req.NewPassword.Or(""),
		CurrentPassword:     req.CurrentPassword.Or(""),
	}
	changed, err := h.users.ChangeProfile(ctx, u, change)
	if err != nil {
		return nil, err
	}
	res := &siteapi.SiteUserResourceHeaders{Response: *h.userResource(changed.User, changed.Pending)}
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
func (h *Handlers) SiteUserEmailChange(ctx context.Context, req *siteapi.SiteEmailChangeInput) error {
	u, err := sessionUser(ctx)
	if err != nil {
		return err
	}
	return h.requests.EmailChange(ctx, u, string(req.NewEmail), req.CurrentPassword)
}

// SiteUserSignOutEverywhere ends every session of the authenticated user, on
// every device, and clears the acting browser's cookie: no session survives.
func (h *Handlers) SiteUserSignOutEverywhere(ctx context.Context) (*siteapi.SiteUserSignOutEverywhereNoContent, error) {
	a := auth.GetSiteAuth(ctx)
	if a == nil {
		return nil, auth.ErrUnauthorized
	}
	if err := h.users.EndSessions(ctx, a.User.ID); err != nil {
		return nil, err
	}
	return &siteapi.SiteUserSignOutEverywhereNoContent{SetCookie: h.sessions.Cleared().String()}, nil
}
