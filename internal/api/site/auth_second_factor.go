package site

import (
	"context"

	siteapi "github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/api/auth"
)

// SiteAuthSecondFactor is the second login step of a User with a Second factor
// (ADR 0020), by the User's Login (userlogin.Module.SecondFactor): it parses the challenge,
// consults the Login throttle (ADR 0025), verifies the code through the Second factor and counts.
// A current TOTP or unused Recovery code starts the session, recorded as `user.login` like a
// one-step login (a Recovery code is also recorded as `user.recovery_code_use`). Wrong
// codes count as failures of the User's address, and while its delay runs even a
// correct code answers 429. A bad challenge has no trusted address, so it is answered
// 401 without counting; the per-IP login cap still applies to the route.
func (h *Handlers) SiteAuthSecondFactor(ctx context.Context, req *siteapi.SiteLoginSecondFactorInput) (*siteapi.SiteLoginResultHeaders, error) {
	res, err := h.login.SecondFactor(ctx, req.Challenge, req.Code)
	if err != nil {
		return nil, err
	}
	cookie, err := h.sessions.Issue(res.Principal, auth.Session{Method: auth.MethodSecondFactor, ProviderLoginAt: res.ProviderLoginAt})
	if err != nil {
		return nil, err
	}
	return sessionResult(cookie), nil
}
