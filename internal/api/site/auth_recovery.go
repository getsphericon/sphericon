package site

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/getsphericon/sphericon/ent"
	siteapi "github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/accounts"
	"github.com/getsphericon/sphericon/internal/authtoken"
	"github.com/getsphericon/sphericon/internal/credentials"
	"github.com/getsphericon/sphericon/internal/db"
	"github.com/getsphericon/sphericon/internal/i18n"
)

// Token lifetimes for the self-service account flows.
const (
	resetTokenTTL       = time.Hour
	verifyTokenTTL      = 24 * time.Hour
	emailChangeTokenTTL = time.Hour
)

// SiteAuthForgotPassword mints a reset token and emails a link. It answers 202
// whether or not the address matches an account and whether or not the per-address
// budget is spent, so the endpoint cannot be used to enumerate registered emails
// (the per-IP cap answers 429 in the rate limit middleware, before this runs). Every
// request does the same work: take a slot of the address's budget in one atomic
// step, look the address up, mint a token and enqueue a mail job. Only a known
// address that was granted a slot gets its mail delivered; for the others the job
// is a no-send one, so response time does not leak existence either.
func (h *Handlers) SiteAuthForgotPassword(ctx context.Context, req *siteapi.SiteForgotPasswordInput) (siteapi.SiteAuthForgotPasswordRes, error) {
	email := strings.TrimSpace(string(req.Email))
	if email == "" {
		return &siteapi.SiteAuthForgotPasswordAccepted{}, nil
	}
	// Unknown addresses spend their budget exactly like known ones.
	granted, err := h.attempts.Take(ctx, accounts.KindPasswordReset, email)
	if err != nil {
		return nil, err
	}
	u, err := h.accounts.UserByEmail(ctx, email)
	if err != nil && !ent.IsNotFound(err) {
		return nil, err
	}
	// Bind the token to the current password hash: once the password is reset the
	// hash changes and the token stops verifying (single use, no extra column). An
	// unknown address gets a throwaway token over the same code path.
	var uid int64
	var binding string
	to := email
	if u != nil {
		uid, binding, to = u.ID, u.PasswordHash, u.Email
	}
	token, err := h.tokens.Mint(authtoken.PurposePasswordReset, uid, binding, resetTokenTTL, nil)
	if err != nil {
		return nil, err
	}
	// Best-effort enqueue (mirrors the welcome email): never fail the request.
	_ = h.sysmail.EnqueuePasswordReset(ctx, to, token, u != nil && granted)
	return &siteapi.SiteAuthForgotPasswordAccepted{}, nil
}

// SiteAuthResetPassword sets a new password from a reset token.
func (h *Handlers) SiteAuthResetPassword(ctx context.Context, req *siteapi.SiteResetPasswordInput) (siteapi.SiteAuthResetPasswordRes, error) {
	if req.Password == "" {
		v := problem(http.StatusBadRequest, i18n.T("errors.password_required", nil))
		return &v, nil
	}
	uid, _, err := h.tokens.Parse(req.Token, authtoken.PurposePasswordReset, h.passwordHashBinding(ctx))
	if err != nil {
		v := problem(http.StatusBadRequest, i18n.T("errors.reset_link_invalid", nil))
		return &v, nil
	}
	hash, err := credentials.HashPassword(req.Password)
	if err != nil {
		return nil, err
	}
	u, verified, err := h.accounts.ResetPassword(ctx, uid, hash)
	if err != nil {
		return nil, err
	}
	if verified {
		_ = h.welcome.EnqueueWelcome(ctx, u.Email, u.Name)
	}
	return &siteapi.SiteAuthResetPasswordOK{}, nil
}

// SiteAuthVerifyEmail completes a registration from its link (ADR 0030): the token
// proves the address, so it stamps the Verified email, sets the password, ends every
// earlier session, creates the User's first Workspace and sends the welcome email.
// It starts no session; the User signs in afterwards. The token is single use (bound
// to the password hash it was minted over) and scoped to the address it was sent to:
// an old link never verifies a changed address.
func (h *Handlers) SiteAuthVerifyEmail(ctx context.Context, req *siteapi.SiteVerifyEmailInput) (siteapi.SiteAuthVerifyEmailRes, error) {
	if req.Password == "" {
		v := problem(http.StatusBadRequest, i18n.T("errors.password_required", nil))
		return &v, nil
	}
	uid, extra, err := h.tokens.Parse(req.Token, authtoken.PurposeEmailVerify, h.passwordHashBinding(ctx))
	if err != nil {
		v := problem(http.StatusBadRequest, i18n.T("errors.verify_link_invalid", nil))
		return &v, nil
	}
	u, err := h.accounts.User(ctx, uid)
	if err != nil || u.EmailVerifiedAt != nil {
		v := problem(http.StatusBadRequest, i18n.T("errors.verify_link_bad", nil))
		return &v, nil
	}
	if extra["email"] != u.Email {
		v := problem(http.StatusBadRequest, i18n.T("errors.verify_link_stale", nil))
		return &v, nil
	}
	hash, err := credentials.HashPassword(req.Password)
	if err != nil {
		return nil, err
	}
	if _, err := h.accounts.VerifyEmail(ctx, uid, hash, u.Name); err != nil {
		return nil, err
	}
	// Welcome email is a platform (transactional) send via the system sender, to an
	// address that is now proven. Best-effort: never fail the verification.
	_ = h.welcome.EnqueueWelcome(ctx, u.Email, u.Name)
	return &siteapi.SiteAuthVerifyEmailOK{}, nil
}

// SiteAuthConfirmEmailChange applies a requested email change once the new
// address is confirmed via the token sent to it. Public: the link may be opened
// from a browser without a session. The change ends every session of the User
// (ADR 0020); when the confirming request carries a valid session of that same
// User, it continues under a fresh token in the same response. The link alone
// never signs anyone in.
func (h *Handlers) SiteAuthConfirmEmailChange(ctx context.Context, req *siteapi.SiteConfirmEmailChangeInput, params siteapi.SiteAuthConfirmEmailChangeParams) (siteapi.SiteAuthConfirmEmailChangeRes, error) {
	uid, extra, err := h.tokens.Parse(req.Token, authtoken.PurposeEmailChange, h.currentEmailBinding(ctx))
	if err != nil {
		v := siteapi.SiteAuthConfirmEmailChangeBadRequest(problem(http.StatusBadRequest, i18n.T("errors.confirm_link_invalid", nil)))
		return &v, nil
	}
	newEmail := strings.TrimSpace(extra["new"])
	if newEmail == "" {
		v := siteapi.SiteAuthConfirmEmailChangeBadRequest(problem(http.StatusBadRequest, i18n.T("errors.confirm_link_bad", nil)))
		return &v, nil
	}
	// Checked before the change: the epoch bump ends this session too.
	acting, err := h.holdsSession(ctx, params.JWT.Or(""), uid)
	if err != nil {
		return nil, err
	}
	// The new address was proven by clicking this link, so it is verified. The
	// unique index also guards the race if the address was taken meanwhile.
	err = h.accounts.ChangeEmail(ctx, uid, newEmail)
	if db.IsUniqueViolation(err) {
		v := siteapi.SiteAuthConfirmEmailChangeConflict(problem(http.StatusConflict, i18n.T("errors.email_in_use", nil)))
		return &v, nil
	}
	if err != nil {
		return nil, err
	}
	res := &siteapi.SiteAuthConfirmEmailChangeOK{}
	if acting {
		u, err := h.accounts.User(ctx, uid)
		if err != nil {
			return nil, err
		}
		cookie, err := h.sessions.Issue(u)
		if err != nil {
			return nil, err
		}
		res.SetCookie = siteapi.NewOptString(cookie.String())
	}
	return res, nil
}

// holdsSession reports whether a raw session token is a current session of the
// User uid: it verifies and carries uid and that User's present epoch.
func (h *Handlers) holdsSession(ctx context.Context, raw string, uid int64) (bool, error) {
	id, epoch, ok := h.sessions.Holder(raw)
	if !ok || id != uid {
		return false, nil
	}
	u, err := h.accounts.User(ctx, uid)
	if err != nil {
		return false, err
	}
	return u.SessionEpoch == epoch, nil
}

// passwordHashBinding resolves a user's current password hash — the binding for
// reset tokens.
func (h *Handlers) passwordHashBinding(ctx context.Context) func(int64) (string, error) {
	return func(id int64) (string, error) {
		u, err := h.accounts.User(ctx, id)
		if err != nil {
			return "", err
		}
		return u.PasswordHash, nil
	}
}

// currentEmailBinding resolves a user's current email — the binding for
// email-change tokens.
func (h *Handlers) currentEmailBinding(ctx context.Context) func(int64) (string, error) {
	return func(id int64) (string, error) {
		u, err := h.accounts.User(ctx, id)
		if err != nil {
			return "", err
		}
		return u.Email, nil
	}
}
