package site

import (
	"context"
	"errors"

	"github.com/getsphericon/sphericon/ent"
	siteapi "github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/accounts"
	"github.com/getsphericon/sphericon/internal/api/auth"
	"github.com/getsphericon/sphericon/internal/login"
	"github.com/getsphericon/sphericon/internal/ratelimit"
	"github.com/getsphericon/sphericon/internal/secondfactor"
)

// SitePasskeyLoginBegin answers the request options for a usernameless Passkey login and sets
// the ceremony cookie. It is anonymous and writes nothing (ADR 0052).
func (h *Handlers) SitePasskeyLoginBegin(context.Context) (siteapi.SitePasskeyLoginBeginRes, error) {
	raw, ceremony, err := h.passkeys.BeginLogin()
	if err != nil {
		return nil, err
	}
	options, err := decodeOptions[siteapi.SitePasskeyRequestOptions](raw)
	if err != nil {
		return nil, err
	}
	return &siteapi.SitePasskeyLoginBegunHeaders{
		SetCookie: h.ceremonyCookie(auth.PasskeyLoginPath, ceremony),
		Response:  siteapi.SitePasskeyLoginBegun{Options: options},
	}, nil
}

// SitePasskeyLoginFinish logs in with the authenticator's response. It runs through the login
// module as its third step: the User is found by the credential id, the Login throttle is
// consulted for that User before anything is verified, and every other refusal (an unknown
// credential, a response that does not verify, a spent or expired challenge, the verification
// gate) is the one 401 and counts nothing. The session is issued through the one issuer, with
// the Passkey method: it replaces the password and the Second factor.
func (h *Handlers) SitePasskeyLoginFinish(ctx context.Context, req *siteapi.SitePasskeyLoginInput, params siteapi.SitePasskeyLoginFinishParams) (siteapi.SitePasskeyLoginFinishRes, error) {
	response, err := credentialJSON(req.Credential)
	if err != nil {
		return nil, err
	}
	res, err := h.login.Passkey(ctx, login.PasskeyAssertion{Ceremony: params.PASSKEY.Or(""), Response: response})
	if err != nil {
		return nil, err
	}
	switch res.Outcome {
	case login.PasskeyThrottled:
		return nil, ratelimit.FromContext(ctx).Delay(ctx, ratelimit.PolicyLoginAccount, res.Limit, res.Wait, res.Now)
	case login.PasskeyAuthenticated:
		cookie, err := h.issueSession(ctx, res.Principal, accounts.LoginPasskey, auth.Session{Method: auth.MethodPasskey, PasskeyCheckAt: h.now()})
		if err != nil {
			return nil, err
		}
		return &siteapi.SiteLoginResultHeaders{
			SetCookie: siteapi.NewOptString(cookie.String()),
			Response:  siteapi.SiteLoginResult{Outcome: siteapi.SiteLoginOutcomeSession},
		}, nil
	}
	return nil, errors.New("login: unhandled passkey outcome")
}

// findPasskeyLogin is the site's PasskeyFind for the login module: the User the response's
// credential id belongs to.
func (h *Handlers) findPasskeyLogin(ctx context.Context, a login.PasskeyAssertion) (login.Pending[*ent.User], error) {
	u, err := h.passkeys.FindLogin(ctx, a.Response)
	if errors.Is(err, secondfactor.ErrPasskeyUnknown) || errors.Is(err, secondfactor.ErrPasskeysUnavailable) {
		return login.Pending[*ent.User]{}, login.ErrPasskeyUnknown
	}
	if err != nil {
		return login.Pending[*ent.User]{}, err
	}
	return login.Pending[*ent.User]{Principal: u, Address: u.Email, Method: accounts.LoginPasskey}, nil
}

// verifyPasskeyLogin is the site's PasskeyVerify for the login module.
func (h *Handlers) verifyPasskeyLogin(ctx context.Context, p login.Pending[*ent.User], a login.PasskeyAssertion) error {
	err := h.passkeys.FinishLogin(ctx, p.Principal, a.Ceremony, a.Response)
	if secondfactor.CeremonyFailed(err) || errors.Is(err, secondfactor.ErrPasskeysUnavailable) {
		return login.ErrPasskeyInvalid
	}
	return err
}
