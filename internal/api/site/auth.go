package site

import (
	"context"
	"net/http"
	"strconv"
	"strings"

	siteapi "github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/authtoken"
	"github.com/getsphericon/sphericon/internal/credentials"
	"github.com/getsphericon/sphericon/internal/db"
	"github.com/getsphericon/sphericon/internal/i18n"
)

func (h *Handlers) SiteAuthRegister(ctx context.Context, req *siteapi.SiteRegisterInput) (siteapi.SiteAuthRegisterRes, error) {
	name := strings.TrimSpace(req.Name)
	email := strings.TrimSpace(string(req.Email))
	password := req.Password

	// Required-field validation is hand-rolled here (and likewise in
	// SiteTokensCreate, SiteWorkspacesUpdate, SiteUserUpdateMe) rather than
	// declared as TypeSpec @minLength on the input models. ogen enforces spec
	// constraints in the request decoder *before* the handler runs, which would
	// collapse these into a generic 400 — untyped in the generated client,
	// since these operations only declare 422 — and drop the per-field error
	// map below. Hand-rolling also lets us TrimSpace and reject blank /
	// whitespace-only values, which @minLength(1) would accept. Keep it here.
	if name == "" || email == "" || password == "" {
		fieldErrors := map[string][]string{}
		if name == "" {
			fieldErrors["name"] = []string{i18n.T("errors.name_required", nil)}
		}
		if email == "" {
			fieldErrors["email"] = []string{i18n.T("errors.email_required", nil)}
		}
		if password == "" {
			fieldErrors["password"] = []string{i18n.T("errors.password_required", nil)}
		}
		v := siteapi.SiteAuthRegisterUnprocessableEntity(problemWithErrors(
			http.StatusUnprocessableEntity,
			i18n.T("errors.register_required", nil),
			fieldErrors,
		))
		return &v, nil
	}

	hash, err := credentials.HashPassword(password)
	if err != nil {
		return nil, err
	}

	u, err := h.accounts.CreateUser(ctx, name, email, hash)
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

	if _, err := h.accounts.CreateWorkspace(ctx, u.ID, name); err != nil {
		return nil, err
	}

	// Welcome email is a platform (transactional) send via the system sender — a
	// river job in prod, run inline in tests. Best-effort: never fail registration.
	_ = h.welcome.EnqueueWelcome(ctx, u.Email, u.Name)

	// Send a (soft) email-verification link. Best-effort, like the welcome email.
	if token, err := h.tokens.Mint(authtoken.PurposeEmailVerify, u.ID, "", verifyTokenTTL, map[string]string{"email": u.Email}); err == nil {
		_ = h.sysmail.EnqueueEmailVerification(ctx, u.Email, token)
	}

	return &siteapi.SiteRegisterResult{
		ID:        siteapi.EntityId(strconv.FormatInt(u.ID, 10)),
		Name:      u.Name,
		Email:     siteapi.EmailAddress(u.Email),
		CreatedAt: siteapi.Timestamp(u.CreatedAt),
	}, nil
}
