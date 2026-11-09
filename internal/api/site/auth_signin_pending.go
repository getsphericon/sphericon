package site

import (
	"context"
	"net/url"

	siteapi "github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/api/auth"
	"github.com/getsphericon/sphericon/internal/domain/identity/signinprovider"
)

// signinPendingPath is the SPA's screen that asks the Sign-up items of a pending sign-up.
const signinPendingPath = "/signin-pending"

// signinHeld is the redirect of a provider sign-up held for the Sign-up items (ADR 0062): to the
// screen that asks them, with the signed reference of the pending sign-up and the page the flow
// began from. No session starts, and the spent flow cookie is cleared.
func (h *Handlers) signinHeld(flow signinprovider.Flow, token string) *siteapi.SiteAuthSigninCallbackFound {
	next := url.Values{"token": {token}}
	if flow.ReturnTo != "" {
		next.Set("redirect", flow.ReturnTo)
	}
	return &siteapi.SiteAuthSigninCallbackFound{
		Location:  signinPendingPath + "?" + next.Encode(),
		SetCookie: siteapi.NewOptString(h.sessions.ClearedSigninFlow().String()),
	}
}

// SiteAuthSigninPending shows the screen of a pending sign-up who the provider proved.
func (h *Handlers) SiteAuthSigninPending(ctx context.Context, params siteapi.SiteAuthSigninPendingParams) (*siteapi.SitePendingSignup, error) {
	pending, err := h.links.PendingSignup(ctx, params.Token)
	if err != nil {
		return nil, err
	}
	res := mapper.PendingSignupToResource(pending)
	res.Provider = h.signin.DisplayName(pending.Provider)
	return res, nil
}

// SiteAuthSigninPendingSubmit completes a pending sign-up with the answers to the Sign-up items
// (ADR 0062): the sign-up use case resolves the reference, checks the answers as the provider
// screen's, creates the User and what comes with it in one transaction and records the login, and
// the session starts as at the end of a provider sign-in, at the time of that sign-in.
func (h *Handlers) SiteAuthSigninPendingSubmit(ctx context.Context, req *siteapi.SiteSigninPendingSubmitInput) (*siteapi.SiteAuthSigninPendingSubmitNoContent, error) {
	done, err := h.signUp.CompletePendingSignup(ctx, req.Token, inputs.SignupAnswers(req.SignupItems))
	if err != nil {
		return nil, err
	}
	held := done.PendingSignup
	cookie, err := h.sessions.Issue(done.User, auth.Session{Method: auth.MethodProvider, ProviderLoginAt: held.CreatedAt})
	if err != nil {
		return nil, err
	}
	return &siteapi.SiteAuthSigninPendingSubmitNoContent{SetCookie: cookie.String()}, nil
}

// SiteAuthSigninPendingDecline deletes a pending sign-up: nothing is created. A reference that
// names nothing open any more is declined already.
func (h *Handlers) SiteAuthSigninPendingDecline(ctx context.Context, req *siteapi.SiteSigninPendingDeclineInput) error {
	return h.links.DeclinePendingSignup(ctx, req.Token)
}
