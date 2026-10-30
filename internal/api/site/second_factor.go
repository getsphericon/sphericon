package site

import (
	"context"
	"errors"
	"net/http"
	"time"

	siteapi "github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/api/auth"
	"github.com/getsphericon/sphericon/internal/i18n"
	"github.com/getsphericon/sphericon/internal/narrow"
	"github.com/getsphericon/sphericon/internal/secondfactor"
)

// SiteSecondFactorGetStatus answers whether the User has a Second factor, a pending
// enrollment, and how many Recovery codes are left (never the codes).
func (h *Handlers) SiteSecondFactorGetStatus(ctx context.Context) (*siteapi.SiteSecondFactorStatus, error) {
	userID, err := sessionUserID(ctx)
	if err != nil {
		return nil, err
	}
	st, err := h.secondFactor.Status(ctx, userID)
	if err != nil {
		return nil, err
	}
	u, err := h.accounts.User(ctx, userID)
	if err != nil {
		return nil, err
	}
	return &siteapi.SiteSecondFactorStatus{
		Enabled: st.Enabled, Pending: st.Pending, RecoveryCodesRemaining: narrow.Int32(st.RecoveryCodesRemaining),
		PasswordSet: u.PasswordHash != "", IdentityProofFresh: u.PasswordHash == "" && h.identityProofFresh(ctx),
	}, nil
}

// SiteSecondFactorStartEnrollment creates a pending TOTP secret (ADR 0020) once the
// password is proven: a hijacked session alone cannot enroll a factor of its own.
func (h *Handlers) SiteSecondFactorStartEnrollment(ctx context.Context, req *siteapi.SiteSecondFactorStartInput) (siteapi.SiteSecondFactorStartEnrollmentRes, error) {
	userID, denied, err := h.proveIdentity(ctx, req.CurrentPassword)
	if err != nil {
		return nil, err
	}
	if denied != nil {
		v := siteapi.SiteSecondFactorStartEnrollmentForbidden(*denied)
		return &v, nil
	}
	e, err := h.secondFactor.StartEnrollment(ctx, userID)
	if err != nil {
		return nil, err
	}
	return &siteapi.SiteSecondFactorEnrollment{Secret: e.Secret, OtpauthUri: e.URI, QrCode: e.QRCode}, nil
}

// SiteSecondFactorConfirmEnrollment activates the pending secret with the password
// and a valid code, returns the Recovery codes once and reissues the acting session
// (the epoch moved).
func (h *Handlers) SiteSecondFactorConfirmEnrollment(ctx context.Context, req *siteapi.SiteSecondFactorConfirmInput) (siteapi.SiteSecondFactorConfirmEnrollmentRes, error) {
	userID, denied, err := h.proveIdentity(ctx, req.CurrentPassword)
	if err != nil {
		return nil, err
	}
	if denied != nil {
		v := siteapi.SiteSecondFactorConfirmEnrollmentForbidden(*denied)
		return &v, nil
	}
	u, codes, err := h.secondFactor.ConfirmEnrollment(ctx, userID, req.Code)
	switch {
	case errors.Is(err, secondfactor.ErrInvalidCode):
		v := siteapi.SiteSecondFactorConfirmEnrollmentUnprocessableEntity(codeInvalidProblem())
		return &v, nil
	case err != nil:
		return nil, err
	}
	cookie, err := h.sessions.Issue(u, currentSession(ctx))
	if err != nil {
		return nil, err
	}
	return &siteapi.SiteRecoveryCodesHeaders{SetCookie: cookie.String(), Response: siteapi.SiteRecoveryCodes{Codes: codes}}, nil
}

// SiteSecondFactorRegenerateRecoveryCodes replaces the Recovery codes after the
// password is proven, and reissues the acting session.
func (h *Handlers) SiteSecondFactorRegenerateRecoveryCodes(ctx context.Context, req *siteapi.SiteRecoveryCodesInput) (siteapi.SiteSecondFactorRegenerateRecoveryCodesRes, error) {
	userID, denied, err := h.proveIdentity(ctx, req.CurrentPassword)
	if err != nil {
		return nil, err
	}
	if denied != nil {
		v := siteapi.SiteSecondFactorRegenerateRecoveryCodesForbidden(*denied)
		return &v, nil
	}
	u, codes, err := h.secondFactor.RegenerateRecoveryCodes(ctx, userID)
	if errors.Is(err, secondfactor.ErrNotActive) {
		v := siteapi.SiteSecondFactorRegenerateRecoveryCodesConflict(problem(http.StatusConflict, i18n.T("errors.second_factor_not_active", nil)))
		return &v, nil
	}
	if err != nil {
		return nil, err
	}
	cookie, err := h.sessions.Issue(u, currentSession(ctx))
	if err != nil {
		return nil, err
	}
	return &siteapi.SiteRecoveryCodesHeaders{SetCookie: cookie.String(), Response: siteapi.SiteRecoveryCodes{Codes: codes}}, nil
}

// SiteSecondFactorDisable removes the Second factor after the password and a
// current code are proven, and reissues the acting session.
func (h *Handlers) SiteSecondFactorDisable(ctx context.Context, req *siteapi.SiteSecondFactorDisableInput) (siteapi.SiteSecondFactorDisableRes, error) {
	userID, denied, err := h.proveIdentity(ctx, req.CurrentPassword)
	if err != nil {
		return nil, err
	}
	if denied != nil {
		v := siteapi.SiteSecondFactorDisableForbidden(*denied)
		return &v, nil
	}
	u, err := h.secondFactor.Disable(ctx, userID, req.Code)
	switch {
	case errors.Is(err, secondfactor.ErrInvalidCode):
		v := siteapi.SiteSecondFactorDisableUnprocessableEntity(codeInvalidProblem())
		return &v, nil
	case errors.Is(err, secondfactor.ErrNotActive):
		v := siteapi.SiteSecondFactorDisableConflict(problem(http.StatusConflict, i18n.T("errors.second_factor_not_active", nil)))
		return &v, nil
	case err != nil:
		return nil, err
	}
	cookie, err := h.sessions.Issue(u, currentSession(ctx).WithoutSecondFactor())
	if err != nil {
		return nil, err
	}
	return &siteapi.SiteSecondFactorDisableNoContent{SetCookie: cookie.String()}, nil
}

// sessionUserID is the id of the User the request's session belongs to.
func sessionUserID(ctx context.Context) (int64, error) {
	a := auth.GetSiteAuth(ctx)
	if a == nil {
		return 0, auth.ErrUnauthorized
	}
	return a.UserID, nil
}

// identityProofTTL is how long a Sign-in provider login (ADR 0048) or a Passkey check (ADR 0052)
// stands in for the password of a User who has none: the proof is the time carried by the
// session, so it needs no store and ends by itself.
const identityProofTTL = 5 * time.Minute

// proveIdentity is the proof the steps that ask for the password need (ADR 0020, ADR 0048).
// A User with a password proves it the way a login does (ADR 0025): while the account's Login
// throttle delay runs it fails with the 429 rejection even for the right password, and a wrong
// password counts as a failure of the account. A right one does not reset the counter: only a
// completed login does, else a session holder could clear a run of second-step guesses. A User
// with no password proves a Sign-in provider login or a Passkey check of the last
// identityProofTTL instead, carried by the session; there is nothing to guess, so nothing is
// counted. denied is the
// problem to answer with when the proof fails, nil when it holds.
func (h *Handlers) proveIdentity(ctx context.Context, password siteapi.OptString) (userID int64, denied *siteapi.ProblemDetails, err error) {
	if userID, err = sessionUserID(ctx); err != nil {
		return 0, nil, err
	}
	u, err := h.accounts.User(ctx, userID)
	if err != nil {
		return 0, nil, err
	}
	if u.PasswordHash == "" {
		if h.identityProofFresh(ctx) {
			return userID, nil, nil
		}
		v := problem(http.StatusForbidden, i18n.T("errors.identity_proof_required", nil))
		return userID, &v, nil
	}
	proved, err := h.proveCurrentPassword(ctx, u, password.Value)
	if err != nil {
		return 0, nil, err
	}
	if proved {
		return userID, nil, nil
	}
	v := passwordIncorrectProblem()
	return userID, &v, nil
}

// identityProofFresh is whether the request's session holds a Sign-in provider login or a
// Passkey check recent enough to count as a proof.
func (h *Handlers) identityProofFresh(ctx context.Context) bool {
	a := auth.GetSiteAuth(ctx)
	if a == nil {
		return false
	}
	return h.fresh(a.ProviderLoginAt) || h.fresh(a.PasskeyCheckAt)
}

func (h *Handlers) fresh(at time.Time) bool {
	if at.IsZero() {
		return false
	}
	age := h.now().Sub(at)
	return age >= 0 && age <= identityProofTTL
}

// currentSession is how the request's session began, to hand to the session that replaces it.
func currentSession(ctx context.Context) auth.Session {
	if a := auth.GetSiteAuth(ctx); a != nil {
		return a.Session
	}
	return auth.Session{}
}

func passwordIncorrectProblem() siteapi.ProblemDetails {
	return problem(http.StatusForbidden, i18n.T("errors.current_password_incorrect", nil))
}

func codeInvalidProblem() siteapi.ProblemDetails {
	msg := i18n.T("errors.second_factor_code_invalid", nil)
	return problemWithErrors(http.StatusUnprocessableEntity, msg, map[string][]string{"code": {msg}})
}
