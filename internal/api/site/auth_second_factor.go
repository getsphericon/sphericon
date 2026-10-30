package site

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/getsphericon/sphericon/ent"
	siteapi "github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/accounts"
	"github.com/getsphericon/sphericon/internal/api/auth"
	"github.com/getsphericon/sphericon/internal/authtoken"
	"github.com/getsphericon/sphericon/internal/login"
	"github.com/getsphericon/sphericon/internal/ratelimit"
	"github.com/getsphericon/sphericon/internal/secondfactor"
)

// loginMethodClaim is the challenge claim that names how the first step proved the person.
const loginMethodClaim = "method"

// loginChallengeTTL is how long the password step's challenge stays valid.
const loginChallengeTTL = 5 * time.Minute

// mintLoginChallenge signs the challenge the password step answers with: bound to
// the User and to secondfactor.ChallengeBinding, so it stops verifying after one
// successful second step (or any epoch bump or password change), with no store.
//
// method (accounts.LoginPassword or a provider's name) rides in the signed claims, so the
// second step records the login with the method that began it. A provider login also passes
// its time (providerLoginAt, zero for a password login), which the challenge carries to the
// session the second step starts (ADR 0048).
func (h *Handlers) mintLoginChallenge(ctx context.Context, u *ent.User, method string, providerLoginAt time.Time) (string, error) {
	binding, err := h.secondFactor.ChallengeBinding(ctx, u)
	if err != nil {
		return "", err
	}
	extra := map[string]string{loginMethodClaim: method}
	if !providerLoginAt.IsZero() {
		extra[challengeProviderLogin] = strconv.FormatInt(providerLoginAt.Unix(), 10)
	}
	return h.challenges.Mint(authtoken.PurposeLoginChallenge, u.ID, binding, loginChallengeTTL, extra)
}

// challengeProviderLogin is the challenge's extra claim that holds the time of the provider
// login it follows.
const challengeProviderLogin = "provider_login"

// SiteAuthSecondFactor is the second login step of a User with a Second factor
// (ADR 0020): the login module parses the challenge, consults the Login throttle (ADR
// 0025), verifies the code through the adapter and counts. A current TOTP or unused
// Recovery code starts the session, recorded as `user.login` like a one-step login (a
// Recovery code is also recorded as `user.recovery_code_use` by the module). Wrong
// codes count as failures of the User's address, and while its delay runs even a
// correct code answers 429. A bad challenge has no trusted address, so it is answered
// 401 without counting; the per-IP login cap still applies to the route.
func (h *Handlers) SiteAuthSecondFactor(ctx context.Context, req *siteapi.SiteLoginSecondFactorInput) (siteapi.SiteAuthSecondFactorRes, error) {
	res, err := h.login.SecondFactor(ctx, req.Challenge, req.Code)
	if err != nil {
		return nil, err
	}
	switch res.Outcome {
	case login.SecondThrottled:
		return nil, ratelimit.FromContext(ctx).Delay(ctx, ratelimit.PolicyLoginAccount, res.Limit, res.Wait, res.Now)
	case login.SecondAuthenticated:
		cookie, err := h.issueSession(ctx, res.Principal, res.Method, auth.Session{Method: auth.MethodSecondFactor, ProviderLoginAt: res.ProviderLoginAt})
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

// verifyLoginCode is the site's code check for the login module: the Second factor
// module's Verify (a TOTP code, or a Recovery code spent once) under the binding the
// challenge was minted with.
func (h *Handlers) verifyLoginCode(ctx context.Context, p login.Pending[*ent.User], code string) error {
	_, err := h.secondFactor.Verify(ctx, p.Principal.ID, p.Binding, code)
	switch {
	case errors.Is(err, secondfactor.ErrInvalidCode):
		return login.ErrInvalidCode
	case errors.Is(err, secondfactor.ErrNotActive), errors.Is(err, secondfactor.ErrBindingMoved):
		// The factor is gone, or a racing second step spent the challenge first.
		return login.ErrChallengeInvalid
	}
	return err
}

// parseLoginChallenge returns the Pending a still-valid challenge was minted for: its
// User, the binding it verified under and the method and provider login time it
// carries. login.ErrChallengeInvalid when it is forged, expired, already used, or its
// User is gone or no longer has a Second factor; other errors are the store's.
func (h *Handlers) parseLoginChallenge(ctx context.Context, challenge string) (login.Pending[*ent.User], error) {
	var (
		u       *ent.User
		binding string
		loadErr error
	)
	_, extra, err := h.challenges.Parse(challenge, authtoken.PurposeLoginChallenge, func(id int64) (string, error) {
		if u, loadErr = h.accounts.User(ctx, id); loadErr != nil {
			return "", loadErr
		}
		binding, loadErr = h.secondFactor.ChallengeBinding(ctx, u)
		return binding, loadErr
	})
	if loadErr != nil && !ent.IsNotFound(loadErr) && !errors.Is(loadErr, secondfactor.ErrNotActive) {
		return login.Pending[*ent.User]{}, loadErr
	}
	if err != nil {
		return login.Pending[*ent.User]{}, login.ErrChallengeInvalid
	}
	var providerLoginAt time.Time
	if sec, perr := strconv.ParseInt(extra[challengeProviderLogin], 10, 64); perr == nil {
		providerLoginAt = time.Unix(sec, 0)
	}
	return login.Pending[*ent.User]{
		Principal:       u,
		Address:         u.Email,
		Binding:         binding,
		Method:          cmp.Or(extra[loginMethodClaim], accounts.LoginPassword),
		ProviderLoginAt: providerLoginAt,
	}, nil
}
