package site

import (
	"context"
	"net/http"
	"strings"

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
		_ = h.sysmail.EnqueueEmailVerification(ctx, u.Email, token)
	}
	return &siteapi.SiteAuthRegisterAccepted{}, nil
}
