package site

import (
	"context"
	"fmt"
	"net/http"

	siteapi "github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/api/auth"
	"github.com/getsphericon/sphericon/internal/domain/identity/login"
	"github.com/getsphericon/sphericon/internal/infra/ratelimit"
)

// SiteAuthLogin is the one route that starts a session (ADR 0020), by the User's Login
// (userlogin.Module.Password). It consults the Login throttle first (ADR 0025): while the
// address's delay runs it answers 429 even for a correct password, else guessing until one
// attempt succeeds would skip the delay. An unknown email, a wrong password and a User without a
// password all answer the same 401, after the same password hash work, and all count as a failure
// of the address. An Unverified User is refused by the gate (ADR 0030, unless the install turned
// it off) with no session, no challenge and nothing counted. The refusals are the login module's
// errors. A success resets the counter, is recorded as `user.login` in each of the User's
// Workspaces (ADR 0022) and sets the session cookie.
//
// A User with an active Second factor gets a challenge instead of the cookie, and
// the counter is not reset: only the second step resets it, else knowing the
// password would buy a fresh round of code guesses on every login.
func (h *Handlers) SiteAuthLogin(ctx context.Context, req *siteapi.SiteLoginInput) (*siteapi.SiteLoginResultHeaders, error) {
	res, err := h.login.Password(ctx, req.Email, req.Password)
	if err != nil {
		return nil, err
	}
	switch res.Outcome {
	case login.Throttled:
		return nil, ratelimit.FromContext(ctx).Delay(ctx, ratelimit.PolicyLoginAccount, res.Limit, res.Wait, res.Now)
	case login.SecondFactor:
		return &siteapi.SiteLoginResultHeaders{Response: siteapi.SiteLoginResult{
			Outcome:   siteapi.SiteLoginOutcomeChallenge,
			Challenge: siteapi.NewOptString(res.Challenge),
		}}, nil
	case login.Authenticated:
		cookie, err := h.sessions.Issue(res.Principal, auth.Session{Method: auth.MethodPassword})
		if err != nil {
			return nil, err
		}
		return sessionResult(cookie), nil
	}
	return nil, fmt.Errorf("login: unhandled outcome %d", res.Outcome)
}

// sessionResult is the answer of a sign-in that started a session: its cookie.
func sessionResult(cookie *http.Cookie) *siteapi.SiteLoginResultHeaders {
	return &siteapi.SiteLoginResultHeaders{
		SetCookie: siteapi.NewOptString(cookie.String()),
		Response:  siteapi.SiteLoginResult{Outcome: siteapi.SiteLoginOutcomeSession},
	}
}

// SiteAuthLogout ends the session on this browser by clearing its cookie. A token
// copied elsewhere stays valid until it expires or the User's epoch is bumped.
func (h *Handlers) SiteAuthLogout(context.Context) (*siteapi.SiteAuthLogoutNoContent, error) {
	return &siteapi.SiteAuthLogoutNoContent{SetCookie: h.sessions.Cleared().String()}, nil
}
