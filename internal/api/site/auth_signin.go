package site

import (
	"cmp"
	"context"
	"errors"
	"log/slog"
	"net/http"
	"net/url"

	"github.com/getsphericon/sphericon/ent"
	siteapi "github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/accounts"
	"github.com/getsphericon/sphericon/internal/i18n"
	"github.com/getsphericon/sphericon/internal/secondfactor"
	"github.com/getsphericon/sphericon/internal/signin"
)

// The pages the callback sends the browser to. The SPA owns both routes.
const (
	signinHomePath  = "/"
	signinLoginPath = "/login"
	// signinInvitationPath is the SPA's page of an Invitation link, followed by its token.
	signinInvitationPath = "/invitations/"
)

// SiteAuthSigninProviders lists the Sign-in providers the Instance enabled, for the buttons
// of the login and register pages (empty when none is configured).
func (h *Handlers) SiteAuthSigninProviders(context.Context) (*siteapi.SiteSigninProviders, error) {
	enabled := h.signin.Enabled()
	items := make([]siteapi.SiteSigninProvider, len(enabled))
	for i, p := range enabled {
		items[i] = siteapi.SiteSigninProvider{Name: p.Name, DisplayName: p.DisplayName}
	}
	return &siteapi.SiteSigninProviders{Items: items}, nil
}

// SiteAuthSigninStart begins a provider sign-in (ADR 0048): it draws the flow, keeps it in
// the signed flow cookie with the `return_to` that survived validation, and redirects the
// browser to the provider.
func (h *Handlers) SiteAuthSigninStart(_ context.Context, params siteapi.SiteAuthSigninStartParams) (siteapi.SiteAuthSigninStartRes, error) {
	authURL, flow, ok := h.signin.Start(params.Provider, signin.SafeReturnTo(params.ReturnTo.Or("")))
	if !ok {
		v := problem(http.StatusNotFound, i18n.T("errors.signin_provider_unknown", nil))
		return &v, nil
	}
	flow.Invitation = params.Invitation.Or("")
	sealed, err := h.signin.SealFlow(flow)
	if err != nil {
		return nil, err
	}
	return &siteapi.SiteAuthSigninStartFound{
		Location:  authURL,
		SetCookie: h.sessions.SigninFlow(sealed, signin.FlowTTL).String(),
	}, nil
}

// SiteAuthSigninCallback is where the provider sends the browser back. Whatever happens it
// answers with a redirect: to the page the flow began from with the session cookie, or to
// the login page with the Reason, which the SPA renders. A callback that is not for a flow
// this browser began (no cookie, another state) is refused before the provider is asked
// anything.
func (h *Handlers) SiteAuthSigninCallback(ctx context.Context, params siteapi.SiteAuthSigninCallbackParams) (*siteapi.SiteAuthSigninCallbackFound, error) {
	if !h.signin.Has(params.Provider) {
		return h.signinRefused(signin.Flow{}, signin.ReasonProviderUnavailable), nil
	}
	flow, err := h.signin.OpenFlow(params.SIGNIN.Or(""), params.Provider)
	if err != nil || !flow.StateMatches(params.State.Or("")) {
		return h.signinRefused(signin.Flow{}, signin.ReasonInvalidState), nil
	}
	if params.Error.Set {
		return h.signinRefused(flow, providerErrorReason(params.Error.Value)), nil
	}
	if params.Code.Or("") == "" {
		return h.signinRefused(flow, signin.ReasonProviderError), nil
	}

	profile, err := h.signin.Complete(ctx, flow, params.Code.Value)
	if err != nil {
		// The error names the provider's failure, never a token or the subject.
		slog.WarnContext(ctx, "signin: the provider's answer was not usable", "provider", params.Provider, "error", err)
		return h.signinRefused(flow, signin.ReasonProviderError), nil
	}

	if flow.LinkUserID != 0 {
		return h.linkProvider(ctx, flow, params.JWT.Or(""), profile)
	}

	var invite *ent.Invitation
	if flow.Invitation != "" {
		invite, err = h.accounts.PendingInvitation(ctx, flow.Invitation)
		if ent.IsNotFound(err) {
			return h.signinRefused(flow, signin.ReasonInvitationInvalid), nil
		}
		if err != nil {
			return nil, err
		}
	}

	signedIn, err := h.accounts.SignInWithIdentity(ctx, accounts.ProviderLogin{
		Provider:     params.Provider,
		Subject:      profile.Subject,
		Email:        profile.Email,
		Name:         profile.Name,
		TrustFailure: h.signin.Trust(params.Provider, profile),
		SignupOpen:   h.signupOpen,
		Invitation:   invite,
	})
	if refused, ok := errors.AsType[*accounts.SigninRefusedError](err); ok {
		return h.signinRefused(flow, refused.Reason), nil
	}
	if err != nil {
		return nil, err
	}
	if signedIn.LinkedIdentityID != 0 {
		h.noticeIdentityLinked(ctx, signedIn.User, params.Provider, signedIn.LinkedIdentityID)
	}
	if secondfactor.Active(signedIn.User) {
		// A provider sign-in replaces only the password step: an enrolled Second factor still
		// gets the challenge of ADR 0020, finished on the login page through the same second
		// step (and the same Login throttle) as after a password.
		challenge, err := h.mintLoginChallenge(ctx, signedIn.User, params.Provider, h.now())
		if err != nil {
			return nil, err
		}
		next := url.Values{"challenge": {challenge}}
		if flow.ReturnTo != "" {
			// The login page sends the person on to the page the flow began from.
			next.Set("redirect", flow.ReturnTo)
		}
		return &siteapi.SiteAuthSigninCallbackFound{
			Location:  signinLoginPath + "?" + next.Encode(),
			SetCookie: siteapi.NewOptString(h.sessions.ClearedSigninFlow().String()),
		}, nil
	}
	cookie, err := h.issueSession(ctx, signedIn.User, params.Provider, h.now())
	if err != nil {
		return nil, err
	}
	return h.signinLanded(flow, cookie.String()), nil
}

// signinLanded is the redirect of a flow that succeeded: to the page it began from, or to the
// home page (the settings page for a link), with the session cookie.
func (h *Handlers) signinLanded(flow signin.Flow, sessionCookie string) *siteapi.SiteAuthSigninCallbackFound {
	return &siteapi.SiteAuthSigninCallbackFound{
		Location:  cmp.Or(flow.ReturnTo, flowPage(flow, signinHomePath)),
		SetCookie: siteapi.NewOptString(sessionCookie),
	}
}

// flowPage is the page a flow lands on by default: the settings page for a link, home for
// a sign-in.
func flowPage(flow signin.Flow, home string) string {
	switch {
	case flow.LinkUserID != 0:
		return signinSettingsPath
	case flow.Invitation != "" && home == signinLoginPath:
		// A refusal of a flow that began on an Invitation lands back on it, which tells the
		// person which address it is for.
		return signinInvitationPath + url.PathEscape(flow.Invitation)
	}
	return home
}

// signinRefused is the redirect of a flow that did not succeed: the login page with the
// Reason (the settings page for a link), and the flow cookie cleared so a finished flow
// leaves nothing behind. A refusal before any flow was opened passes the zero Flow.
func (h *Handlers) signinRefused(flow signin.Flow, reason signin.Reason) *siteapi.SiteAuthSigninCallbackFound {
	return &siteapi.SiteAuthSigninCallbackFound{
		Location:  flowPage(flow, signinLoginPath) + "?" + url.Values{"error": {string(reason)}}.Encode(),
		SetCookie: siteapi.NewOptString(h.sessions.ClearedSigninFlow().String()),
	}
}

// providerErrorReason maps the `error` a provider sent back (RFC 6749 section 4.1.2.1) to the
// Reason shown: a person who said no is told so, anything else is the provider's failure.
func providerErrorReason(code string) signin.Reason {
	if code == "access_denied" {
		return signin.ReasonDenied
	}
	return signin.ReasonProviderError
}
