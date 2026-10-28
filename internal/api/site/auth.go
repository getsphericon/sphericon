package site

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/getsphericon/sphericon/ent"
	siteapi "github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/accounts"
	"github.com/getsphericon/sphericon/internal/api/auth"
	"github.com/getsphericon/sphericon/internal/authtoken"
	"github.com/getsphericon/sphericon/internal/credentials"
	"github.com/getsphericon/sphericon/internal/db"
	"github.com/getsphericon/sphericon/internal/i18n"
)

// SiteAuthRegister creates an Unverified User from a name, an email and a password
// (ADR 0030): no session, no Workspace. The password is stored hashed, and the mailed
// token is bound to it and to the browser that registered, whose cookie the answer sets;
// the link trusts the password only when that browser opens it (SiteAuthVerifyEmail).
// The Workspace and the welcome email follow the proof.
func (h *Handlers) SiteAuthRegister(ctx context.Context, req *siteapi.SiteRegisterInput) (siteapi.SiteAuthRegisterRes, error) {
	open, err := h.accounts.SignupOpen(ctx)
	if err != nil {
		return nil, err
	}
	if !open {
		return signupClosed(), nil
	}
	name := strings.TrimSpace(req.Name)
	email := strings.TrimSpace(string(req.Email))

	// Required-field validation is done here (and likewise, by hand, in
	// SiteTokensCreate, SiteWorkspacesUpdate, SiteUserUpdateMe) rather than
	// declared as TypeSpec @minLength on the input models. ogen enforces spec
	// constraints in the request decoder *before* the handler runs, which would
	// collapse these into a generic 400 — untyped in the generated client,
	// since these operations only declare 422 — and drop the per-field error
	// map below. Checking here also lets us TrimSpace and reject blank /
	// whitespace-only values, which @minLength(1) would accept. Keep it here.
	password := req.Password.Or("")
	fieldErrors, err := checkFields(registerForm{Name: name, Email: email, Password: password})
	if err != nil {
		return nil, err
	}
	if fieldErrors != nil {
		v := siteapi.SiteAuthRegisterUnprocessableEntity(fieldsProblem(fieldErrors))
		return &v, nil
	}

	if !h.requireVerification {
		return h.registerVerified(ctx, name, email, password)
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
	// The password is hashed whatever the address is, so the work does not say whether
	// it is Taken; a Verified User's hash is dropped unused.
	hash, err := credentials.HashPassword(password)
	if err != nil {
		return nil, err
	}
	// replace is whether the address already holds a User whose password this registration
	// replaces: every path but the one that creates it.
	replace := true
	u, err := h.accounts.UserByEmail(ctx, email)
	if ent.IsNotFound(err) {
		u, err = h.accounts.Register(ctx, name, email, hash, h.signupSurvey.Enabled())
		replace = false
		if db.IsUniqueViolation(err) {
			// Two registrations of one new address raced: the loser finds the winner's row
			// and follows the existing-address path below, so a race is not an answer of its
			// own. It replaces the winner's password too (ADR 0030 notes the consequence).
			u, err = h.accounts.UserByEmail(ctx, email)
			replace = true
		}
	}
	if err != nil {
		return nil, err
	}
	if replace {
		// A repeated registration of an Unverified address replaces its password, which
		// ends the earlier mails; a Verified User's row is left as it is.
		if u, err = h.accounts.ReplaceUnverifiedPassword(ctx, u.ID, hash); err != nil {
			return nil, err
		}
	}

	// The token is bound to the stored password hash, so it works until a registration
	// replaces the hash or the address is verified, and it carries the hash of the
	// registration cookie set below. A Verified User gets the same token and cookie, and
	// its notice carries no token at all. Best-effort send; a lost mail is asked for again.
	cookie, browser := h.sessions.NewRegistrationBrowser(verifyTokenTTL)
	token, err := h.tokens.Mint(authtoken.PurposeEmailVerify, u.ID, u.PasswordHash, verifyTokenTTL,
		auth.VerifyClaims(u.Email, browser))
	if err != nil {
		return nil, err
	}
	if u.EmailVerifiedAt != nil {
		if err := h.sysmail.EnqueueAddressTakenNotice(ctx, u.Email, granted); err != nil {
			mailNotQueued(ctx, "AddressTakenNotice", err)
		}
	} else {
		if err := h.sysmail.EnqueueEmailVerification(ctx, u.Email, token, granted); err != nil {
			mailNotQueued(ctx, "EmailVerification", err)
		}
	}
	return &siteapi.SiteAuthRegisterAccepted{SetCookie: siteapi.NewOptString(cookie.String())}, nil
}

// registerForm is what SiteAuthRegister checks beyond the contract, the values already trimmed.
type registerForm struct {
	Name     string `json:"name" validate:"required"`
	Email    string `json:"email" validate:"required,account_email"`
	Password string `json:"password" validate:"required,password_not_blank,password_length"`
}

// registerVerified is registration with the gate off (ADR 0030): the password comes
// with the request, so the User is Verified with its first Workspace at once, and the
// welcome email follows as it does after a verification link. No session starts; the
// User signs in next.
func (h *Handlers) registerVerified(ctx context.Context, name, email, password string) (siteapi.SiteAuthRegisterRes, error) {
	hash, err := credentials.HashPassword(password)
	if err != nil {
		return nil, err
	}
	u, err := h.accounts.SignUpVerified(ctx, name, email, hash, h.signupSurvey.Enabled())
	if errors.Is(err, accounts.ErrSignupClosed) {
		return signupClosed(), nil // another registration took the first place meanwhile
	}
	if db.IsUniqueViolation(err) {
		return emailTaken(), nil
	}
	if err != nil {
		return nil, err
	}
	if err := h.welcome.EnqueueWelcome(ctx, u.Email, u.Name); err != nil {
		mailNotQueued(ctx, "Welcome", err)
	}
	return &siteapi.SiteAuthRegisterAccepted{}, nil
}

// signupClosed is the 403 answer to a registration the Instance's sign-up rule refuses
// (ADR 0060); accepting an Invitation still works.
func signupClosed() siteapi.SiteAuthRegisterRes {
	v := siteapi.SiteAuthRegisterForbidden(siteapi.ProblemDetails{
		Code:   siteapi.NewOptProblemCode(siteapi.ProblemCodeSignupClosed),
		Status: siteapi.NewOptInt32(http.StatusForbidden),
		Title:  siteapi.NewOptString(http.StatusText(http.StatusForbidden)),
		Detail: siteapi.NewOptString(i18n.T("errors.signup_closed", nil)),
	})
	return &v
}

// emailTaken is the 409 answer to a registration with an address already in use.
func emailTaken() siteapi.SiteAuthRegisterRes {
	v := siteapi.SiteAuthRegisterConflict(problemWithErrors(
		http.StatusConflict,
		i18n.T("errors.email_exists", nil),
		map[string][]string{fieldEmail: {i18n.T("errors.email_exists", nil)}},
	))
	return &v
}

// SiteAuthResendVerification sends the verification link again to an Unverified User
// (ADR 0030). It answers 202 the same way for an unknown, an already verified and an
// unverified address, and every request does the same work (a token is minted, a mail
// job is queued); only an Unverified User's job delivers, the others' is a no-send one,
// so neither the answer nor its timing says whether the address has an account. The
// per-IP cap answers 429 in the rate limit middleware, before this runs.
func (h *Handlers) SiteAuthResendVerification(ctx context.Context, req *siteapi.SiteResendVerificationInput, params siteapi.SiteAuthResendVerificationParams) (siteapi.SiteAuthResendVerificationRes, error) {
	email := strings.TrimSpace(string(req.Email))
	if email == "" {
		return &siteapi.SiteAuthResendVerificationAccepted{}, nil
	}
	u, err := h.accounts.UserByEmail(ctx, email)
	if err != nil && !ent.IsNotFound(err) {
		return nil, err
	}
	// The same binding as at registration: the link works until the password is replaced
	// or the address verified. It carries the hash of the registration cookie the request
	// holds, none otherwise (then the link asks for a password); every request hashes the
	// same way, so a cookie changes no timing.
	var uid int64
	var binding string
	to := email
	if u != nil {
		uid, binding, to = u.ID, u.PasswordHash, u.Email
	}
	// The cookie is SameSite=Strict: the browser withholds it on the cross-site click from
	// the mail, but sends it on the page's own request that follows (same site), which is
	// the one that carries the token.
	claims := auth.VerifyClaims(to, auth.RegistrationHash(params.REG.Or("")))
	token, err := h.tokens.Mint(authtoken.PurposeEmailVerify, uid, binding, verifyTokenTTL, claims)
	if err != nil {
		return nil, err
	}
	if err := h.sysmail.EnqueueEmailVerification(ctx, to, token, u != nil && u.EmailVerifiedAt == nil); err != nil {
		mailNotQueued(ctx, "EmailVerification", err)
	}
	return &siteapi.SiteAuthResendVerificationAccepted{}, nil
}
