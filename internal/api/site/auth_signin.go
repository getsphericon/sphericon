package site

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"net/url"

	siteapi "github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/api/auth"
	"github.com/getsphericon/sphericon/internal/domain/identity/signinprovider"
	"github.com/getsphericon/sphericon/internal/domain/identity/users"
	"github.com/getsphericon/sphericon/internal/usecase/signup"
)

// The pages the callback sends the browser to. The SPA owns both routes.
const (
	signinHomePath  = "/"
	signinLoginPath = "/login"
	// signinInvitationPath is the SPA's page of an Invitation link, followed by its token.
	signinInvitationPath = "/invitations/"
)

// SiteAuthSigninProviders lists the Sign-in providers the Instance enabled, for the buttons
// of the login and sign up pages (empty when none is configured).
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
func (h *Handlers) SiteAuthSigninStart(_ context.Context, params siteapi.SiteAuthSigninStartParams) (*siteapi.SiteAuthSigninStartFound, error) {
	authURL, flow, err := h.signin.Start(params.Provider, signinprovider.SafeReturnTo(params.ReturnTo.Or("")))
	if err != nil {
		return nil, err
	}
	flow.Invitation = params.Invitation.Or("")
	sealed, err := h.signin.SealFlow(flow)
	if err != nil {
		return nil, err
	}
	return &siteapi.SiteAuthSigninStartFound{
		Location:  authURL,
		SetCookie: h.sessions.SigninFlow(sealed, signinprovider.FlowTTL).String(),
	}, nil
}

// SiteAuthSigninCallback is where the provider sends the browser back. Whatever happens it
// answers with a redirect: to the page the flow began from with the session cookie, or to
// the login page with the Reason, which the SPA renders. A callback that is not for a flow
// this browser began (no cookie, another state) is refused before the provider is asked
// anything. The sign-in itself is the use case's (signup.UseCase.SignInWithProvider); what it
// did becomes the cookie or the redirect here.
func (h *Handlers) SiteAuthSigninCallback(ctx context.Context, params siteapi.SiteAuthSigninCallbackParams) (*siteapi.SiteAuthSigninCallbackFound, error) {
	if !h.signin.Has(params.Provider) {
		return h.signinRefused(signinprovider.Flow{}, signinprovider.ReasonProviderUnavailable), nil
	}
	flow, err := h.signin.OpenFlow(params.SIGNIN.Or(""), params.Provider)
	if err != nil || !flow.StateMatches(params.State.Or("")) {
		return h.signinRefused(signinprovider.Flow{}, signinprovider.ReasonInvalidState), nil
	}
	if params.Error.Set {
		return h.signinRefused(flow, providerErrorReason(params.Error.Value)), nil
	}
	if params.Code.Or("") == "" {
		return h.signinRefused(flow, signinprovider.ReasonProviderError), nil
	}

	session := params.JWT.Or("")
	in := inputs.SigninCallback(params)
	in.Flow = flow
	if id, epoch, ok := h.sessions.Holder(session); ok {
		in.Session = signup.SessionHolder{UserID: id, Epoch: epoch}
	}
	res, err := h.signUp.SignInWithProvider(ctx, in)
	if refused, ok := errors.AsType[*users.SigninRefusedError](err); ok {
		return h.signinRefused(flow, refused.Reason), nil
	}
	if err != nil {
		return nil, err
	}
	switch res.Outcome {
	case signup.PendingSignupKept:
		token, err := h.pendingReference(res.Pending)
		if err != nil {
			return nil, err
		}
		return h.signinHeld(flow, token), nil
	case signup.SecondFactorDue:
		// The login page finishes it through the same second step as after a password.
		challenge, err := h.login.Challenge(res.User, res.ChallengeBinding, flow.Provider, res.At)
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
	case signup.IdentityLinked:
		// Signing in again with a provider account the User already had is a fresh provider
		// login. A provider account added now proves nothing about the person (a stolen session
		// could add its own), so the session keeps the login time it had; every other session
		// goes on.
		sess, _ := h.sessions.SessionOf(session)
		if !res.Added {
			sess.ProviderLoginAt = res.At
		}
		return h.signinLanded(flow, sess, res)
	case signup.SignedIn:
		return h.signinLanded(flow, auth.Session{Method: auth.MethodProvider, ProviderLoginAt: res.At}, res)
	}
	return nil, fmt.Errorf("signin: unhandled outcome %d", res.Outcome)
}

// signinLanded is the redirect of a flow that succeeded: the session sess of the User the
// sign-in landed on, to the page the flow began from, or to the home page (the settings page
// for a link).
func (h *Handlers) signinLanded(flow signinprovider.Flow, sess auth.Session, res *signup.ProviderSignInResult) (*siteapi.SiteAuthSigninCallbackFound, error) {
	cookie, err := h.sessions.Issue(res.User, sess)
	if err != nil {
		return nil, err
	}
	return &siteapi.SiteAuthSigninCallbackFound{
		Location:  cmp.Or(flow.ReturnTo, flowPage(flow, signinHomePath)),
		SetCookie: siteapi.NewOptString(cookie.String()),
	}, nil
}

// flowPage is the page a flow lands on by default: the settings page for a link, home for
// a sign-in.
func flowPage(flow signinprovider.Flow, home string) string {
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
func (h *Handlers) signinRefused(flow signinprovider.Flow, reason signinprovider.Reason) *siteapi.SiteAuthSigninCallbackFound {
	return &siteapi.SiteAuthSigninCallbackFound{
		Location:  flowPage(flow, signinLoginPath) + "?" + url.Values{"error": {string(reason)}}.Encode(),
		SetCookie: siteapi.NewOptString(h.sessions.ClearedSigninFlow().String()),
	}
}

// providerErrorReason maps the `error` a provider sent back (RFC 6749 section 4.1.2.1) to the
// Reason shown: a person who said no is told so, anything else is the provider's failure.
func providerErrorReason(code string) signinprovider.Reason {
	if code == "access_denied" {
		return signinprovider.ReasonDenied
	}
	return signinprovider.ReasonProviderError
}
