package site

import (
	"context"
	"errors"
	"net/http"

	siteapi "github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/accounts"
	"github.com/getsphericon/sphericon/internal/api/auth"
	"github.com/getsphericon/sphericon/internal/credentials"
	"github.com/getsphericon/sphericon/internal/i18n"
	"github.com/getsphericon/sphericon/internal/ratelimit"
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
	return &siteapi.SiteSecondFactorStatus{
		Enabled: st.Enabled, Pending: st.Pending, RecoveryCodesRemaining: int32(st.RecoveryCodesRemaining),
	}, nil
}

// SiteSecondFactorStartEnrollment creates a pending TOTP secret (ADR 0020) once the
// password is proven: a hijacked session alone cannot enroll a factor of its own.
func (h *Handlers) SiteSecondFactorStartEnrollment(ctx context.Context, req *siteapi.SiteSecondFactorStartInput) (siteapi.SiteSecondFactorStartEnrollmentRes, error) {
	userID, ok, err := h.provePassword(ctx, req.CurrentPassword)
	if err != nil {
		return nil, err
	}
	if !ok {
		v := siteapi.SiteSecondFactorStartEnrollmentForbidden(passwordIncorrectProblem())
		return &v, nil
	}
	e, err := h.secondFactor.StartEnrollment(ctx, userID)
	if errors.Is(err, secondfactor.ErrAlreadyActive) {
		v := siteapi.SiteSecondFactorStartEnrollmentConflict(problem(http.StatusConflict, i18n.T("errors.second_factor_already_active", nil)))
		return &v, nil
	}
	if err != nil {
		return nil, err
	}
	return &siteapi.SiteSecondFactorEnrollment{Secret: e.Secret, OtpauthUri: e.URI, QrCode: e.QRCode}, nil
}

// SiteSecondFactorConfirmEnrollment activates the pending secret with the password
// and a valid code, returns the Recovery codes once and reissues the acting session
// (the epoch moved).
func (h *Handlers) SiteSecondFactorConfirmEnrollment(ctx context.Context, req *siteapi.SiteSecondFactorConfirmInput) (siteapi.SiteSecondFactorConfirmEnrollmentRes, error) {
	userID, ok, err := h.provePassword(ctx, req.CurrentPassword)
	if err != nil {
		return nil, err
	}
	if !ok {
		v := siteapi.SiteSecondFactorConfirmEnrollmentForbidden(passwordIncorrectProblem())
		return &v, nil
	}
	u, codes, err := h.secondFactor.ConfirmEnrollment(ctx, userID, req.Code)
	switch {
	case errors.Is(err, secondfactor.ErrInvalidCode):
		v := siteapi.SiteSecondFactorConfirmEnrollmentUnprocessableEntity(codeInvalidProblem())
		return &v, nil
	case errors.Is(err, secondfactor.ErrNoPending):
		v := siteapi.SiteSecondFactorConfirmEnrollmentConflict(problem(http.StatusConflict, i18n.T("errors.second_factor_no_pending", nil)))
		return &v, nil
	case errors.Is(err, secondfactor.ErrAlreadyActive):
		v := siteapi.SiteSecondFactorConfirmEnrollmentConflict(problem(http.StatusConflict, i18n.T("errors.second_factor_already_active", nil)))
		return &v, nil
	case err != nil:
		return nil, err
	}
	cookie, err := h.sessions.Issue(u)
	if err != nil {
		return nil, err
	}
	return &siteapi.SiteRecoveryCodesHeaders{SetCookie: cookie.String(), Response: siteapi.SiteRecoveryCodes{Codes: codes}}, nil
}

// SiteSecondFactorRegenerateRecoveryCodes replaces the Recovery codes after the
// password is proven, and reissues the acting session.
func (h *Handlers) SiteSecondFactorRegenerateRecoveryCodes(ctx context.Context, req *siteapi.SiteRecoveryCodesInput) (siteapi.SiteSecondFactorRegenerateRecoveryCodesRes, error) {
	userID, ok, err := h.provePassword(ctx, req.CurrentPassword)
	if err != nil {
		return nil, err
	}
	if !ok {
		v := siteapi.SiteSecondFactorRegenerateRecoveryCodesForbidden(passwordIncorrectProblem())
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
	cookie, err := h.sessions.Issue(u)
	if err != nil {
		return nil, err
	}
	return &siteapi.SiteRecoveryCodesHeaders{SetCookie: cookie.String(), Response: siteapi.SiteRecoveryCodes{Codes: codes}}, nil
}

// SiteSecondFactorDisable removes the Second factor after the password and a
// current code are proven, and reissues the acting session.
func (h *Handlers) SiteSecondFactorDisable(ctx context.Context, req *siteapi.SiteSecondFactorDisableInput) (siteapi.SiteSecondFactorDisableRes, error) {
	userID, ok, err := h.provePassword(ctx, req.CurrentPassword)
	if err != nil {
		return nil, err
	}
	if !ok {
		v := siteapi.SiteSecondFactorDisableForbidden(passwordIncorrectProblem())
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
	cookie, err := h.sessions.Issue(u)
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

// provePassword checks the session User's current password the way a login does
// (ADR 0020, ADR 0025): while the account's Login throttle delay runs it fails with
// the 429 rejection even for the right password, and a wrong password counts as a
// failure of the account. A right one does not reset the counter: only a completed
// login does, else a session holder could clear a run of second-step guesses. ok is
// false for a wrong password.
func (h *Handlers) provePassword(ctx context.Context, password string) (userID int64, ok bool, err error) {
	if userID, err = sessionUserID(ctx); err != nil {
		return 0, false, err
	}
	u, err := h.accounts.User(ctx, userID)
	if err != nil {
		return 0, false, err
	}
	wait, err := h.attempts.Delay(ctx, accounts.KindLogin, u.Email)
	if err != nil {
		return 0, false, err
	}
	if wait > 0 {
		return 0, false, ratelimit.FromContext(ctx).Delay(ctx, ratelimit.PolicyLoginAccount,
			h.attempts.Limit(accounts.KindLogin), wait, h.attempts.Now())
	}
	if u.PasswordHash != "" && credentials.VerifyPassword(u.PasswordHash, password) {
		return userID, true, nil
	}
	return userID, false, h.attempts.RecordFailure(ctx, accounts.KindLogin, u.Email)
}

func passwordIncorrectProblem() siteapi.ProblemDetails {
	return problem(http.StatusForbidden, i18n.T("errors.current_password_incorrect", nil))
}

func codeInvalidProblem() siteapi.ProblemDetails {
	msg := i18n.T("errors.second_factor_code_invalid", nil)
	return problemWithErrors(http.StatusUnprocessableEntity, msg, map[string][]string{"code": {msg}})
}
