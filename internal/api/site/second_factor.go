package site

import (
	"context"

	"github.com/getsphericon/sphericon/ent"
	siteapi "github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/api/auth"
	"github.com/getsphericon/sphericon/internal/domain/identity/users"
	"github.com/getsphericon/sphericon/internal/kernel/narrow"
)

// SiteSecondFactorGetStatus answers whether the User has a Second factor, a pending
// enrollment, and how many Recovery codes are left (never the codes).
func (h *Handlers) SiteSecondFactorGetStatus(ctx context.Context) (*siteapi.SiteSecondFactorStatus, error) {
	u, err := sessionUser(ctx)
	if err != nil {
		return nil, err
	}
	st, err := h.secondFactor.Status(ctx, u.ID)
	if err != nil {
		return nil, err
	}
	return &siteapi.SiteSecondFactorStatus{
		Enabled: st.Enabled, Pending: st.Pending, RecoveryCodesRemaining: narrow.Int32(st.RecoveryCodesRemaining),
		PasswordSet: u.PasswordHash != "", IdentityProofFresh: sessionProof(ctx, "").StandsInFor(u, h.now()),
	}, nil
}

// SiteSecondFactorStartEnrollment creates a pending TOTP secret (ADR 0020) once the User proves
// their identity (the module's step): a hijacked session alone cannot enroll a factor of its own.
func (h *Handlers) SiteSecondFactorStartEnrollment(ctx context.Context, req *siteapi.SiteSecondFactorStartInput) (*siteapi.SiteSecondFactorEnrollment, error) {
	u, proof, err := h.identityProof(ctx, req.CurrentPassword)
	if err != nil {
		return nil, err
	}
	e, err := h.secondFactor.StartEnrollment(ctx, u, proof)
	if err != nil {
		return nil, err
	}
	return &siteapi.SiteSecondFactorEnrollment{EnrollmentId: siteapi.EntityId(e.ID), Secret: e.Secret, OtpauthUri: e.URI, QrCode: e.QRCode}, nil
}

// SiteSecondFactorConfirmEnrollment activates the pending secret with the password
// and a valid code, returns the Recovery codes once and reissues the acting session
// (the epoch moved).
func (h *Handlers) SiteSecondFactorConfirmEnrollment(ctx context.Context, req *siteapi.SiteSecondFactorConfirmInput) (*siteapi.SiteRecoveryCodesHeaders, error) {
	u, proof, err := h.identityProof(ctx, req.CurrentPassword)
	if err != nil {
		return nil, err
	}
	issued, err := h.secondFactor.ConfirmEnrollment(ctx, u, proof, inputs.SecondFactorConfirm(req))
	if err != nil {
		return nil, err
	}
	cookie, err := h.sessions.Issue(issued.User, currentSession(ctx))
	if err != nil {
		return nil, err
	}
	return &siteapi.SiteRecoveryCodesHeaders{SetCookie: cookie.String(), Response: siteapi.SiteRecoveryCodes{Codes: issued.RecoveryCodes}}, nil
}

// SiteSecondFactorRegenerateRecoveryCodes replaces the Recovery codes after the
// password is proven, and reissues the acting session.
func (h *Handlers) SiteSecondFactorRegenerateRecoveryCodes(ctx context.Context, req *siteapi.SiteRecoveryCodesInput) (*siteapi.SiteRecoveryCodesHeaders, error) {
	u, proof, err := h.identityProof(ctx, req.CurrentPassword)
	if err != nil {
		return nil, err
	}
	issued, err := h.secondFactor.RegenerateRecoveryCodes(ctx, u, proof)
	if err != nil {
		return nil, err
	}
	cookie, err := h.sessions.Issue(issued.User, currentSession(ctx))
	if err != nil {
		return nil, err
	}
	return &siteapi.SiteRecoveryCodesHeaders{SetCookie: cookie.String(), Response: siteapi.SiteRecoveryCodes{Codes: issued.RecoveryCodes}}, nil
}

// SiteSecondFactorDisable removes the Second factor after the password and a
// current code are proven, and reissues the acting session.
func (h *Handlers) SiteSecondFactorDisable(ctx context.Context, req *siteapi.SiteSecondFactorDisableInput) (*siteapi.SiteSecondFactorDisableNoContent, error) {
	u, proof, err := h.identityProof(ctx, req.CurrentPassword)
	if err != nil {
		return nil, err
	}
	saved, err := h.secondFactor.Disable(ctx, u, proof, req.Code)
	if err != nil {
		return nil, err
	}
	cookie, err := h.sessions.Issue(saved, currentSession(ctx).WithoutSecondFactor())
	if err != nil {
		return nil, err
	}
	return &siteapi.SiteSecondFactorDisableNoContent{SetCookie: cookie.String()}, nil
}

// sessionUserID is the id of the User the request's session belongs to.
func sessionUserID(ctx context.Context) (int64, error) {
	u, err := sessionUser(ctx)
	if err != nil {
		return 0, err
	}
	return u.ID, nil
}

// sessionUser is the User the request's session belongs to, as the security handler loaded it:
// the actor is read from the context, never loaded again (ADR 0111).
func sessionUser(ctx context.Context) (*ent.User, error) {
	a := auth.GetSiteAuth(ctx)
	if a == nil {
		return nil, auth.ErrUnauthorized
	}
	return a.User, nil
}

// identityProof is the User of the session and the proof of identity they offer for a step that
// asks for the password (users.Module.ProveIdentity, ADR 0020, ADR 0048): the password typed, and
// the times of the session's Sign-in provider login and Passkey check. The step's module proves it.
func (h *Handlers) identityProof(ctx context.Context, password siteapi.OptString) (*ent.User, users.IdentityProof, error) {
	u, err := sessionUser(ctx)
	if err != nil {
		return nil, users.IdentityProof{}, err
	}
	return u, sessionProof(ctx, password.Value), nil
}

// sessionProof is the proof of identity the request's session holds, with the password typed.
func sessionProof(ctx context.Context, password string) users.IdentityProof {
	proof := users.IdentityProof{Password: password}
	if a := auth.GetSiteAuth(ctx); a != nil {
		proof.ProviderLoginAt, proof.PasskeyCheckAt = a.ProviderLoginAt, a.PasskeyCheckAt
	}
	return proof
}

// currentSession is how the request's session began, to hand to the session that replaces it.
func currentSession(ctx context.Context) auth.Session {
	if a := auth.GetSiteAuth(ctx); a != nil {
		return a.Session
	}
	return auth.Session{}
}
