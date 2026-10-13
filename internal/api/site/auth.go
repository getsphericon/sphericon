package site

import (
	"context"
	"net/http"
	"strings"

	"github.com/getsphericon/sphericon/ent"
	siteapi "github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/accounts"
	"github.com/getsphericon/sphericon/internal/authtoken"
	"github.com/getsphericon/sphericon/internal/credentials"
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

	if !h.requireVerification {
		return h.registerVerified(ctx, name, email, req.Password.Or(""))
	}

	// Every request does the same work whatever the address is (ADR 0031): take a slot
	// of its budget, create the User or find the one that holds the address, mint a
	// token and queue exactly one mail job. Only what the job carries differs, and a
	// job with no mail due is a no-send one, so neither the answer nor its timing says
	// whether the address was Taken.
	granted, err := h.attempts.Take(ctx, accounts.KindRegister, email)
	if err != nil {
		return nil, err
	}
	u, err := h.accounts.UserByEmail(ctx, email)
	if ent.IsNotFound(err) {
		u, err = h.accounts.Register(ctx, name, email)
		if db.IsUniqueViolation(err) {
			// Two registrations of one new address raced: the loser finds the winner's row
			// and follows the same path, so a race is not an answer of its own.
			u, err = h.accounts.UserByEmail(ctx, email)
		}
	}
	if err != nil {
		return nil, err
	}

	// The token is bound to the User's password hash (empty while Unverified), so it
	// works once: setting the password ends it. A Verified User's hash is a real one,
	// and its notice carries no token at all. Best-effort send; a lost mail is asked
	// for again.
	token, err := h.tokens.Mint(authtoken.PurposeEmailVerify, u.ID, u.PasswordHash, verifyTokenTTL, map[string]string{"email": u.Email})
	if err != nil {
		return nil, err
	}
	if u.EmailVerifiedAt != nil {
		_ = h.sysmail.EnqueueAddressTakenNotice(ctx, u.Email, granted)
	} else {
		_ = h.sysmail.EnqueueEmailVerification(ctx, u.Email, token, granted)
	}
	return &siteapi.SiteAuthRegisterAccepted{}, nil
}

// registerVerified is registration with the gate off (ADR 0030): the password comes
// with the request, so the User is Verified with its first Workspace at once, and the
// welcome email follows as it does after a verification link. No session starts; the
// User signs in next.
func (h *Handlers) registerVerified(ctx context.Context, name, email, password string) (siteapi.SiteAuthRegisterRes, error) {
	if password == "" {
		v := siteapi.SiteAuthRegisterUnprocessableEntity(problemWithErrors(
			http.StatusUnprocessableEntity,
			i18n.T("errors.password_required", nil),
			map[string][]string{"password": {i18n.T("errors.password_required", nil)}},
		))
		return &v, nil
	}
	hash, err := credentials.HashPassword(password)
	if err != nil {
		return nil, err
	}
	u, err := h.accounts.RegisterVerified(ctx, name, email, hash)
	if db.IsUniqueViolation(err) {
		return emailTaken(), nil
	}
	if err != nil {
		return nil, err
	}
	_ = h.welcome.EnqueueWelcome(ctx, u.Email, u.Name)
	return &siteapi.SiteAuthRegisterAccepted{}, nil
}

// emailTaken is the 409 answer to a registration with an address already in use.
func emailTaken() siteapi.SiteAuthRegisterRes {
	v := siteapi.SiteAuthRegisterConflict(problemWithErrors(
		http.StatusConflict,
		i18n.T("errors.email_exists", nil),
		map[string][]string{"email": {i18n.T("errors.email_exists", nil)}},
	))
	return &v
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
