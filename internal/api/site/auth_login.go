package site

import (
	"context"
	"log/slog"
	"net/http"
	"strings"
	"sync"

	"github.com/getsphericon/sphericon/ent"
	siteapi "github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/accounts"
	"github.com/getsphericon/sphericon/internal/credentials"
	"github.com/getsphericon/sphericon/internal/i18n"
	"github.com/getsphericon/sphericon/internal/ratelimit"
	"github.com/getsphericon/sphericon/internal/secondfactor"
)

// SiteAuthLogin is the one route that starts a session (ADR 0020). It consults the
// Login throttle first (ADR 0025): while the address's delay runs it answers 429
// even for a correct password, else guessing until one attempt succeeds would skip
// the delay. An unknown email, a wrong password and a User without a password all
// answer the same 401, after the same password hash work, and all count as a
// failure of the address. A success resets the counter, sets the session cookie and
// is recorded as `user.login` in each of the User's Workspaces (ADR 0022).
//
// A User with an active Second factor gets a challenge instead of the cookie, and
// the counter is not reset: only the second step resets it, else knowing the
// password would buy a fresh round of code guesses on every login.
func (h *Handlers) SiteAuthLogin(ctx context.Context, req *siteapi.SiteLoginInput) (siteapi.SiteAuthLoginRes, error) {
	email := strings.TrimSpace(req.Email)
	wait, err := h.attempts.Delay(ctx, accounts.KindLogin, email)
	if err != nil {
		return nil, err
	}
	if wait > 0 {
		return nil, ratelimit.FromContext(ctx).Delay(ctx, ratelimit.PolicyLoginAccount,
			h.attempts.Limit(accounts.KindLogin), wait, h.attempts.Now())
	}

	u, ok, err := h.checkPassword(ctx, email, req.Password)
	if err != nil {
		return nil, err
	}
	if !ok {
		if err := h.attempts.RecordFailure(ctx, accounts.KindLogin, email); err != nil {
			return nil, err
		}
		v := problem(http.StatusUnauthorized, i18n.T("errors.invalid_credentials", nil))
		return &v, nil
	}
	if secondfactor.Active(u) {
		challenge, err := h.mintLoginChallenge(ctx, u)
		if err != nil {
			return nil, err
		}
		return &siteapi.SiteLoginResultHeaders{Response: siteapi.SiteLoginResult{
			Outcome:   siteapi.SiteLoginOutcomeChallenge,
			Challenge: siteapi.NewOptString(challenge),
		}}, nil
	}
	return h.startSession(ctx, u)
}

// startSession ends a successful login: it resets the address's counter, issues
// the session cookie and records `user.login`.
func (h *Handlers) startSession(ctx context.Context, u *ent.User) (*siteapi.SiteLoginResultHeaders, error) {
	if err := h.attempts.RecordSuccess(ctx, accounts.KindLogin, u.Email); err != nil {
		return nil, err
	}
	cookie, err := h.sessions.Issue(u)
	if err != nil {
		return nil, err
	}
	if err := h.accounts.RecordLogin(ctx, u); err != nil {
		// The session is already granted; the entry cannot veto it.
		slog.ErrorContext(ctx, "audit: login not recorded", "error", err)
	}
	return &siteapi.SiteLoginResultHeaders{
		SetCookie: siteapi.NewOptString(cookie.String()),
		Response:  siteapi.SiteLoginResult{Outcome: siteapi.SiteLoginOutcomeSession},
	}, nil
}

// SiteAuthLogout ends the session on this browser by clearing its cookie. A token
// copied elsewhere stays valid until it expires or the User's epoch is bumped.
func (h *Handlers) SiteAuthLogout(context.Context) (*siteapi.SiteAuthLogoutNoContent, error) {
	return &siteapi.SiteAuthLogoutNoContent{SetCookie: h.sessions.Cleared().String()}, nil
}

// checkPassword returns the User the credentials belong to; ok is false when they
// match none. A password is always hashed, against a throwaway hash when there is no
// User or the User has no password, so the answer time does not tell an unknown
// email apart.
func (h *Handlers) checkPassword(ctx context.Context, email, password string) (u *ent.User, ok bool, err error) {
	u, err = h.accounts.UserByEmail(ctx, email)
	if err != nil && !ent.IsNotFound(err) {
		return nil, false, err
	}
	if u == nil || u.PasswordHash == "" {
		credentials.VerifyPassword(decoyHash(), password)
		return nil, false, nil
	}
	return u, credentials.VerifyPassword(u.PasswordHash, password), nil
}

// decoyHash is a real password hash nobody knows the password of, hashed once.
var decoyHash = sync.OnceValue(func() string {
	h, err := credentials.HashPassword("decoy password: no account matches")
	if err != nil {
		return ""
	}
	return h
})
