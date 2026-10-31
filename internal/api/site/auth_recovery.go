package site

import (
	"context"
	"strings"

	"github.com/getsphericon/sphericon/ent"
	siteapi "github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/accounts"
	"github.com/getsphericon/sphericon/internal/api/auth"
	"github.com/getsphericon/sphericon/internal/mailjobs"
)

// SiteAuthForgotPassword mints a reset token and emails a link. It answers 202
// whether or not the address matches an account and whether or not the per-address
// budget is spent, so the endpoint cannot be used to enumerate the addresses that hold a User
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
	// The link is bound to the current password hash, so it works once; an unknown address
	// gets a throwaway one over the same code path.
	to := email
	if u != nil {
		to = u.Email
	}
	token, err := h.links.PasswordResetLink(u)
	if err != nil {
		return nil, err
	}
	// Best-effort enqueue (mirrors the welcome email): never fail the request.
	if err := h.mail.Enqueue(ctx, mailjobs.PasswordReset(to, token, u != nil && granted)); err != nil {
		mailNotQueued(ctx, "PasswordReset", err)
	}
	return &siteapi.SiteAuthForgotPasswordAccepted{}, nil
}

// SiteAuthResetPassword sets a new password from a reset link (accounts.Links).
func (h *Handlers) SiteAuthResetPassword(ctx context.Context, req *siteapi.SiteResetPasswordInput) (siteapi.SiteAuthResetPasswordRes, error) {
	u, completedSignUp, err := h.links.ResetPassword(ctx, req.Token, req.Password)
	if err != nil {
		return nil, err
	}
	if completedSignUp {
		if err := h.mail.Enqueue(ctx, mailjobs.Welcome(u.Email, u.Name)); err != nil {
			mailNotQueued(ctx, "Welcome", err)
		}
	}
	return &siteapi.SiteAuthResetPasswordOK{}, nil
}

// SiteAuthVerifyEmail completes a Sign-up from its link (ADR 0030, accounts.Links): the
// token proves the address, so it stamps the Verified email, ends every earlier session,
// creates the User's first Workspace and sends the welcome email, then starts the
// session through the login issuer. The password stored at Sign-up stays only when
// the request carries the Sign-up browser cookie the token was minted for; any other
// browser must send a password, which replaces it.
func (h *Handlers) SiteAuthVerifyEmail(ctx context.Context, req *siteapi.SiteVerifyEmailInput, params siteapi.SiteAuthVerifyEmailParams) (siteapi.SiteAuthVerifyEmailRes, error) {
	verified, landing, err := h.links.VerifyEmail(ctx, req.Token, params.SIGNUPBROWSER.Or(""), req.Password.Or(""))
	if err != nil {
		return nil, err
	}
	// Welcome email is a platform (transactional) send via the system sender, to an
	// address that is now proven. Best-effort: never fail the verification.
	if err := h.mail.Enqueue(ctx, mailjobs.Welcome(verified.Email, verified.Name)); err != nil {
		mailNotQueued(ctx, "Welcome", err)
	}
	// The session starts after the epoch bump VerifyEmail made, so it is the only one.
	cookie, err := h.sessions.Issue(verified, auth.Session{Method: auth.MethodEmail})
	if err != nil {
		return nil, err
	}
	return &siteapi.SiteVerifyEmailResultHeaders{
		SetCookie: cookie.String(),
		Response:  siteapi.SiteVerifyEmailResult{LandingWorkspaceId: siteapi.WorkspaceId(landing.PublicID)},
	}, nil
}

// SiteAuthConfirmEmailChange applies a requested email change once the new
// address is confirmed via the token sent to it. Public: the link may be opened
// from a browser without a session. The change ends every session of the User
// (ADR 0020); when the confirming request carries a valid session of that same
// User, it continues under a fresh token in the same response. The link alone
// never signs anyone in.
func (h *Handlers) SiteAuthConfirmEmailChange(ctx context.Context, req *siteapi.SiteConfirmEmailChangeInput, params siteapi.SiteAuthConfirmEmailChangeParams) (siteapi.SiteAuthConfirmEmailChangeRes, error) {
	change, err := h.links.EmailChange(ctx, req.Token)
	if err != nil {
		return nil, err
	}
	uid := change.UserID
	// Checked before the change: the epoch bump ends this session too.
	acting, err := h.holdsSession(ctx, params.JWT.Or(""), uid)
	if err != nil {
		return nil, err
	}
	// A token that records no known method is an invalid session: it is not continued.
	sess, valid := h.sessions.SessionOf(params.JWT.Or(""))
	acting = acting && valid
	if err := h.links.ConfirmEmailChange(ctx, change); err != nil {
		return nil, err
	}
	res := &siteapi.SiteAuthConfirmEmailChangeOK{}
	if acting {
		u, err := h.accounts.User(ctx, uid)
		if err != nil {
			return nil, err
		}
		cookie, err := h.sessions.Issue(u, sess)
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
