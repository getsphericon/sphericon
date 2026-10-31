package site

import (
	"context"
	"errors"
	"net/url"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/consentrecord"
	siteapi "github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/accounts"
	"github.com/getsphericon/sphericon/internal/api/auth"
	"github.com/getsphericon/sphericon/internal/authtoken"
	"github.com/getsphericon/sphericon/internal/signin"
)

// signinPendingPath is the SPA's screen that asks the Sign-up items of a pending sign-up.
const signinPendingPath = "/signin-pending"

// signinHeld is the redirect of a provider sign-up held for the Sign-up items (ADR 0062): to the
// screen that asks them, with the signed reference of the pending sign-up and the page the flow
// began from. No session starts, and the spent flow cookie is cleared.
func (h *Handlers) signinHeld(flow signin.Flow, token string) *siteapi.SiteAuthSigninCallbackFound {
	next := url.Values{"token": {token}}
	if flow.ReturnTo != "" {
		next.Set("redirect", flow.ReturnTo)
	}
	return &siteapi.SiteAuthSigninCallbackFound{
		Location:  signinPendingPath + "?" + next.Encode(),
		SetCookie: siteapi.NewOptString(h.sessions.ClearedSigninFlow().String()),
	}
}

// pendingReference is the signed reference of a pending sign-up: valid as long as the pending
// sign-up is open, and keyed to its provider subject, so it names nothing once the row is gone.
func (h *Handlers) pendingReference(pending *ent.PendingSignup) (string, error) {
	return h.challenges.Mint(authtoken.PurposePendingSignup, pending.ID, pending.Subject, accounts.PendingSignupTTL, nil)
}

// openPending is the pending sign-up a signed reference names while it is open, or
// accounts.ErrPendingSignupGone for a reference that is forged, expired or names nothing open.
func (h *Handlers) openPending(ctx context.Context, token string) (*ent.PendingSignup, error) {
	var pending *ent.PendingSignup
	_, _, err := h.challenges.Parse(token, authtoken.PurposePendingSignup, func(id int64) (string, error) {
		p, err := h.accounts.PendingSignup(ctx, id, h.now())
		if err != nil {
			return "", err
		}
		pending = p
		return p.Subject, nil
	})
	if bindingErr, ok := errors.AsType[*authtoken.BindingError](err); ok && !errors.Is(bindingErr, accounts.ErrPendingSignupGone) {
		return nil, bindingErr.Err
	}
	if err != nil {
		return nil, accounts.ErrPendingSignupGone
	}
	return pending, nil
}

// SiteAuthSigninPending shows the screen of a pending sign-up who the provider proved.
func (h *Handlers) SiteAuthSigninPending(ctx context.Context, params siteapi.SiteAuthSigninPendingParams) (siteapi.SiteAuthSigninPendingRes, error) {
	pending, err := h.openPending(ctx, params.Token)
	if err != nil {
		return nil, err
	}
	res := mapper.PendingSignupToResource(pending)
	res.Provider = h.providerDisplayName(pending.Provider)
	return res, nil
}

// SiteAuthSigninPendingSubmit completes a pending sign-up with the answers to the Sign-up items
// (ADR 0062): the answers are checked as the sign-up form's are, the accounts module creates the
// User and what comes with it in one transaction, and the session starts as at the end of a
// provider sign-in, at the time of that sign-in.
func (h *Handlers) SiteAuthSigninPendingSubmit(ctx context.Context, req *siteapi.SiteSigninPendingSubmitInput) (siteapi.SiteAuthSigninPendingSubmitRes, error) {
	pending, err := h.openPending(ctx, req.Token)
	if err != nil {
		return nil, err
	}
	consents, err := h.consentRecords.Check(ctx, inputs.SignupAnswers(req.SignupItems), consentrecord.SourceProviderScreen)
	if err != nil {
		return nil, err
	}
	u, held, err := h.accounts.CompletePendingSignup(ctx, pending.ID, consents, h.signupSurvey.Enabled(), h.now())
	if err != nil {
		return nil, err
	}
	cookie, err := h.issueSession(ctx, u, held.Provider, auth.Session{Method: auth.MethodProvider, ProviderLoginAt: held.CreatedAt})
	if err != nil {
		return nil, err
	}
	return &siteapi.SiteAuthSigninPendingSubmitNoContent{SetCookie: cookie.String()}, nil
}

// SiteAuthSigninPendingDecline deletes a pending sign-up: nothing is created. A reference that
// names nothing open any more is declined already.
func (h *Handlers) SiteAuthSigninPendingDecline(ctx context.Context, req *siteapi.SiteSigninPendingDeclineInput) error {
	pending, err := h.openPending(ctx, req.Token)
	if errors.Is(err, accounts.ErrPendingSignupGone) {
		return nil
	}
	if err != nil {
		return err
	}
	return h.accounts.DeclinePendingSignup(ctx, pending.ID)
}
