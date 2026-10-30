package site

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/getsphericon/sphericon/ent"
	siteapi "github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/accounts"
	"github.com/getsphericon/sphericon/internal/api/auth"
	"github.com/getsphericon/sphericon/internal/login"
	"github.com/getsphericon/sphericon/internal/ratelimit"
)

// SiteAuthLogin is the one route that starts a session (ADR 0020). It consults the
// Login throttle first (ADR 0025): while the address's delay runs it answers 429
// even for a correct password, else guessing until one attempt succeeds would skip
// the delay. An unknown email, a wrong password and a User without a password all
// answer the same 401, after the same password hash work, and all count as a
// failure of the address. An Unverified User is refused by the gate (ADR 0030, unless the
// install turned it off) with no session, no challenge and nothing counted. The refusals are
// the login module's errors. A success resets the counter, sets the session cookie and is
// recorded as `user.login` in each of the User's Workspaces (ADR 0022).
//
// A User with an active Second factor gets a challenge instead of the cookie, and
// the counter is not reset: only the second step resets it, else knowing the
// password would buy a fresh round of code guesses on every login.
func (h *Handlers) SiteAuthLogin(ctx context.Context, req *siteapi.SiteLoginInput) (siteapi.SiteAuthLoginRes, error) {
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
		cookie, err := h.issueSession(ctx, res.Principal, accounts.LoginPassword, auth.Session{Method: auth.MethodPassword})
		if err != nil {
			return nil, err
		}
		return &siteapi.SiteLoginResultHeaders{
			SetCookie: siteapi.NewOptString(cookie.String()),
			Response:  siteapi.SiteLoginResult{Outcome: siteapi.SiteLoginOutcomeSession},
		}, nil
	}
	return nil, fmt.Errorf("login: unhandled outcome %d", res.Outcome)
}

// loginStore is the site's side of the login module's seam: Users by address.
type loginStore struct{ accounts *accounts.Accounts }

func (s loginStore) Lookup(ctx context.Context, address string) (login.Account[*ent.User], bool, error) {
	u, err := s.accounts.UserByLoginAddress(ctx, address)
	if ent.IsNotFound(err) {
		return login.Account[*ent.User]{}, false, nil
	}
	if err != nil {
		return login.Account[*ent.User]{}, false, err
	}
	return login.Account[*ent.User]{Principal: u, PasswordHash: u.PasswordHash, SecondFactor: accounts.HasSecondFactor(u)}, true, nil
}

// newLogin builds the login module over the site's adapter (ADR 0020, 0030).
func (h *Handlers) newLogin() *login.Module[*ent.User] {
	cfg := login.Config[*ent.User]{
		Throttle:  h.attempts.For(accounts.KindLogin),
		Store:     loginStore{h.accounts},
		Passwords: h.passwords,
		Challenge: func(ctx context.Context, u *ent.User) (string, error) {
			return h.mintLoginChallenge(ctx, u, accounts.LoginPassword, time.Time{})
		},
		Parse:         h.parseLoginChallenge,
		Verify:        h.verifyLoginCode,
		PasskeyFind:   h.findPasskeyLogin,
		PasskeyVerify: h.verifyPasskeyLogin,
	}
	if h.requireVerification {
		cfg.Gate = func(u *ent.User) bool { return u.EmailVerifiedAt == nil }
	}
	return login.New(cfg)
}

// issueSession is the one place a successful sign-in, by password or by a Sign-in provider,
// is granted: it mints the session cookie and records `user.login`. The session is already
// granted when the entry is written, so a failed entry cannot veto it.
func (h *Handlers) issueSession(ctx context.Context, u *ent.User, method string, sess auth.Session) (*http.Cookie, error) {
	cookie, err := h.sessions.Issue(u, sess)
	if err != nil {
		return nil, err
	}
	if err := h.accounts.RecordLogin(ctx, u, method); err != nil {
		slog.ErrorContext(ctx, "audit: login not recorded", "error", err)
	}
	return cookie, nil
}

// SiteAuthLogout ends the session on this browser by clearing its cookie. A token
// copied elsewhere stays valid until it expires or the User's epoch is bumped.
func (h *Handlers) SiteAuthLogout(context.Context) (*siteapi.SiteAuthLogoutNoContent, error) {
	return &siteapi.SiteAuthLogoutNoContent{SetCookie: h.sessions.Cleared().String()}, nil
}
