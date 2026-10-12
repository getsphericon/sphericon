package site

import (
	"context"
	"net/http"
	"strings"

	"github.com/getsphericon/sphericon/ent"
	siteapi "github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/authtoken"
	"github.com/getsphericon/sphericon/internal/db"
	"github.com/getsphericon/sphericon/internal/i18n"
)

// SiteAuthRegister creates an Unverified User from a name and an email (ADR 0030):
// no password, no session, no Workspace. The verification email carries the link
// where the address is proven and the password is set; the Workspace and the welcome
// email follow that proof (SiteAuthVerifyEmail).
func (h *Handlers) SiteAuthRegister(ctx context.Context, req *siteapi.SiteRegisterInput) (siteapi.SiteAuthRegisterRes, error) {
	name := strings.TrimSpace(req.Name)
	email := strings.TrimSpace(string(req.Email))

	// Required-field validation is hand-rolled here (and likewise in
	// SiteTokensCreate, SiteWorkspacesUpdate, SiteUserUpdateMe) rather than
	// declared as TypeSpec @minLength on the input models. ogen enforces spec
	// constraints in the request decoder *before* the handler runs, which would
	// collapse these into a generic 400 — untyped in the generated client,
	// since these operations only declare 422 — and drop the per-field error
	// map below. Hand-rolling also lets us TrimSpace and reject blank /
	// whitespace-only values, which @minLength(1) would accept. Keep it here.
	if name == "" || email == "" {
		fieldErrors := map[string][]string{}
		if name == "" {
			fieldErrors["name"] = []string{i18n.T("errors.name_required", nil)}
		}
		if email == "" {
			fieldErrors["email"] = []string{i18n.T("errors.email_required", nil)}
		}
		v := siteapi.SiteAuthRegisterUnprocessableEntity(problemWithErrors(
			http.StatusUnprocessableEntity,
			i18n.T("errors.register_required", nil),
			fieldErrors,
		))
		return &v, nil
	}

	u, err := h.accounts.Register(ctx, name, email)
	if db.IsUniqueViolation(err) {
		v := siteapi.SiteAuthRegisterConflict(problemWithErrors(
			http.StatusConflict,
			i18n.T("errors.email_exists", nil),
			map[string][]string{"email": {i18n.T("errors.email_exists", nil)}},
		))
		return &v, nil
	}
	if err != nil {
		return nil, err
	}

	// The token is bound to the (still empty) password hash, so it works once: setting
	// the password ends it. Best-effort send; a lost mail is asked for again.
	if token, err := h.tokens.Mint(authtoken.PurposeEmailVerify, u.ID, u.PasswordHash, verifyTokenTTL, map[string]string{"email": u.Email}); err == nil {
		_ = h.sysmail.EnqueueEmailVerification(ctx, u.Email, token, true)
	}
	return &siteapi.SiteAuthRegisterAccepted{}, nil
}

// SiteAuthResendVerification sends the verification link again to an Unverified User
// (ADR 0030). It answers 202 the same way for an unknown, an already verified and an
// unverified address, and every request does the same work (a token is minted, a mail
// job is queued); only an Unverified User's job delivers, the others' is a no-send one,
// so neither the answer nor its timing says whether the address has an account. The
// per-IP cap answers 429 in the rate limit middleware, before this runs.
func (h *Handlers) SiteAuthResendVerification(ctx context.Context, req *siteapi.SiteResendVerificationInput) (siteapi.SiteAuthResendVerificationRes, error) {
	email := strings.TrimSpace(string(req.Email))
	if email == "" {
		return &siteapi.SiteAuthResendVerificationAccepted{}, nil
	}
	u, err := h.accounts.UserByEmail(ctx, email)
	if err != nil && !ent.IsNotFound(err) {
		return nil, err
	}
	// The same binding as at registration: the link works until the password is set.
	var uid int64
	var binding string
	to := email
	if u != nil {
		uid, binding, to = u.ID, u.PasswordHash, u.Email
	}
	token, err := h.tokens.Mint(authtoken.PurposeEmailVerify, uid, binding, verifyTokenTTL, map[string]string{"email": to})
	if err != nil {
		return nil, err
	}
	_ = h.sysmail.EnqueueEmailVerification(ctx, to, token, u != nil && u.EmailVerifiedAt == nil)
	return &siteapi.SiteAuthResendVerificationAccepted{}, nil
}
