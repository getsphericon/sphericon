package site

import (
	"cmp"
	"context"
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/getsphericon/sphericon/ent"
	siteapi "github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/accounts"
	"github.com/getsphericon/sphericon/internal/authtoken"
	"github.com/getsphericon/sphericon/internal/i18n"
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
// (ADR 0020): a valid challenge and a current TOTP or unused Recovery code start the
// session, recorded as `user.login` like a one-step login (a Recovery code is also
// recorded as `user.recovery_code_use` by the module). Wrong codes count as failures
// of the User's address in the Login throttle (ADR 0025), and while its delay runs
// even a correct code answers 429. A bad challenge has no trusted address, so it is
// answered 401 without counting; the per-IP login cap still applies to the route.
func (h *Handlers) SiteAuthSecondFactor(ctx context.Context, req *siteapi.SiteLoginSecondFactorInput) (siteapi.SiteAuthSecondFactorRes, error) {
	u, binding, method, providerLoginAt, err := h.parseLoginChallenge(ctx, req.Challenge)
	if errors.Is(err, errChallengeInvalid) {
		v := problem(http.StatusUnauthorized, i18n.T("errors.login_challenge_invalid", nil))
		return &v, nil
	}
	if err != nil {
		return nil, err
	}

	wait, err := h.attempts.Delay(ctx, accounts.KindLogin, u.Email)
	if err != nil {
		return nil, err
	}
	if wait > 0 {
		return nil, ratelimit.FromContext(ctx).Delay(ctx, ratelimit.PolicyLoginAccount,
			h.attempts.Limit(accounts.KindLogin), wait, h.attempts.Now())
	}

	_, err = h.secondFactor.Verify(ctx, u.ID, binding, req.Code)
	switch {
	case errors.Is(err, secondfactor.ErrInvalidCode):
		if err := h.attempts.RecordFailure(ctx, accounts.KindLogin, u.Email); err != nil {
			return nil, err
		}
		v := problem(http.StatusUnauthorized, i18n.T("errors.second_factor_code_invalid", nil))
		return &v, nil
	case errors.Is(err, secondfactor.ErrNotActive), errors.Is(err, secondfactor.ErrBindingMoved):
		// The factor is gone, or a racing second step spent the challenge first.
		v := problem(http.StatusUnauthorized, i18n.T("errors.login_challenge_invalid", nil))
		return &v, nil
	case err != nil:
		return nil, err
	}
	return h.startSession(ctx, u, method, providerLoginAt)
}

var errChallengeInvalid = errors.New("site: login challenge invalid")

// parseLoginChallenge returns the User a still-valid challenge was minted for,
// the challenge binding it verified under and the provider login time it carries (zero for a
// password login), which the second step re-checks inside
// its transaction. errChallengeInvalid when it is forged, expired, already used,
// or its User is gone or no longer has a Second factor; other errors are the
// store's.
func (h *Handlers) parseLoginChallenge(ctx context.Context, challenge string) (*ent.User, string, string, time.Time, error) {
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
		return nil, "", "", time.Time{}, loadErr
	}
	if err != nil {
		return nil, "", "", time.Time{}, errChallengeInvalid
	}
	var providerLoginAt time.Time
	if sec, perr := strconv.ParseInt(extra[challengeProviderLogin], 10, 64); perr == nil {
		providerLoginAt = time.Unix(sec, 0)
	}
	return u, binding, cmp.Or(extra[loginMethodClaim], accounts.LoginPassword), providerLoginAt, nil
}
