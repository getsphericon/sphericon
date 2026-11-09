package site

import (
	"context"
	"errors"

	siteapi "github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/api/auth"
	"github.com/getsphericon/sphericon/internal/domain/identity/login"
	"github.com/getsphericon/sphericon/internal/infra/ratelimit"
)

// SitePasskeyLoginBegin answers the request options for a usernameless Passkey login and sets
// the ceremony cookie. It is anonymous and writes nothing (ADR 0052).
func (h *Handlers) SitePasskeyLoginBegin(context.Context) (*siteapi.SitePasskeyLoginBegunHeaders, error) {
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

// SitePasskeyLoginFinish logs in with the authenticator's response. It runs through the User's
// Login (userlogin.Module.Passkey), the login module's third step: the User is found by the credential id, the Login throttle is
// consulted for that User before anything is verified, and every other refusal (an unknown
// credential, a response that does not verify, a spent or expired challenge, the verification
// gate) is the one 401 and counts nothing. The login is recorded by Passkey and the session issued
// with the Passkey method: it replaces the password and the Second factor.
func (h *Handlers) SitePasskeyLoginFinish(ctx context.Context, req *siteapi.SitePasskeyLoginInput, params siteapi.SitePasskeyLoginFinishParams) (*siteapi.SiteLoginResultHeaders, error) {
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
		cookie, err := h.sessions.Issue(res.Principal, auth.Session{Method: auth.MethodPasskey, PasskeyCheckAt: h.now()})
		if err != nil {
			return nil, err
		}
		return sessionResult(cookie), nil
	}
	return nil, errors.New("login: unhandled passkey outcome")
}
