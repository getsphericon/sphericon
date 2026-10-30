package site

import (
	"context"
	"time"

	siteapi "github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/accounts"
	"github.com/getsphericon/sphericon/internal/api/auth"
	"github.com/getsphericon/sphericon/internal/narrow"
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
	userID, err := h.proveIdentity(ctx, req.CurrentPassword)
	if err != nil {
		return nil, err
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
	userID, err := h.proveIdentity(ctx, req.CurrentPassword)
	if err != nil {
		return nil, err
	}
	u, codes, err := h.secondFactor.ConfirmEnrollment(ctx, userID, req.Code)
	if err != nil {
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
	userID, err := h.proveIdentity(ctx, req.CurrentPassword)
	if err != nil {
		return nil, err
	}
	u, codes, err := h.secondFactor.RegenerateRecoveryCodes(ctx, userID)
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
	userID, err := h.proveIdentity(ctx, req.CurrentPassword)
	if err != nil {
		return nil, err
	}
	u, err := h.secondFactor.Disable(ctx, userID, req.Code)
	if err != nil {
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

// proveIdentity is the proof the steps that ask for the password need (accounts.ProveIdentity,
// ADR 0020, ADR 0048) for the User of the session: the password through the login module's one
// prove path, or for a User with none a Sign-in provider login or a Passkey check of the last
// identityProofTTL, carried by the session. It returns the User's id once the proof holds, and
// the refusal otherwise.
func (h *Handlers) proveIdentity(ctx context.Context, password siteapi.OptString) (int64, error) {
	userID, err := sessionUserID(ctx)
	if err != nil {
		return 0, err
	}
	u, err := h.accounts.User(ctx, userID)
	if err != nil {
		return 0, err
	}
	if err := accounts.ProveIdentity(ctx, u, password.Value, h.identityProofFresh(ctx), h.proveCurrentPassword); err != nil {
		return 0, err
	}
	return userID, nil
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
