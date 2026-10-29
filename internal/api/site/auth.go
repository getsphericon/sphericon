package site

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/getsphericon/sphericon/ent"
	siteapi "github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/accounts"
	"github.com/getsphericon/sphericon/internal/api/auth"
	"github.com/getsphericon/sphericon/internal/authtoken"
	"github.com/getsphericon/sphericon/internal/db"
	"github.com/getsphericon/sphericon/internal/i18n"
	"github.com/getsphericon/sphericon/internal/signup"
	"github.com/getsphericon/sphericon/internal/validation"
)

// SiteAuthSignUp is the Sign-up (ADR 0030): the signup module decides, and the answer
// carries its outcome and, when a verification link follows, the Sign-up browser's cookie
// (whatever the address, ADR 0031). No session starts and no Workspace is made here; they
// follow the proof.
func (h *Handlers) SiteAuthSignUp(ctx context.Context, req *siteapi.SiteSignUpInput) (siteapi.SiteAuthSignUpRes, error) {
	res, err := h.signup.SignUp(ctx, signup.Input{Name: req.Name, Email: string(req.Email), Password: req.Password.Or("")})
	if fields, ok := errors.AsType[*validation.FieldsError](err); ok {
		v := siteapi.SiteAuthSignUpUnprocessableEntity(fieldsProblem(fields.Fields))
		return &v, nil
	}
	if err != nil {
		return nil, err
	}
	switch res.Outcome {
	case signup.Closed:
		return signupClosed(), nil
	case signup.Taken:
		return emailTaken(), nil
	case signup.Accepted:
		accepted := &siteapi.SiteAuthSignUpAccepted{}
		if res.Browser != "" {
			accepted.SetCookie = siteapi.NewOptString(h.sessions.SignUpBrowser(res.Browser, verifyTokenTTL).String())
		}
		return accepted, nil
	}
	return nil, fmt.Errorf("signup: unhandled outcome %d", res.Outcome)
}

// signUpStore is the site's side of the signup module's seam: Users through accounts.
type signUpStore struct{ accounts *accounts.Accounts }

func (s signUpStore) Open(ctx context.Context) (bool, error) { return s.accounts.SignupOpen(ctx) }

func (s signUpStore) Lookup(ctx context.Context, email string) (*ent.User, bool, error) {
	u, err := s.accounts.UserByEmail(ctx, email)
	if ent.IsNotFound(err) {
		return nil, false, nil
	}
	return u, err == nil, err
}

func (s signUpStore) CreateUnverified(ctx context.Context, name, email, passwordHash string, surveyDue bool) (*ent.User, error) {
	u, err := s.accounts.CreateUnverified(ctx, name, email, passwordHash, surveyDue)
	if db.IsUniqueViolation(err) {
		return nil, signup.ErrTaken
	}
	return u, err
}

func (s signUpStore) ReplaceUnverifiedPassword(ctx context.Context, id int64, passwordHash string) (*ent.User, error) {
	return s.accounts.ReplaceUnverifiedPassword(ctx, id, passwordHash)
}

func (s signUpStore) CreateVerified(ctx context.Context, name, email, passwordHash string, surveyDue bool) (*ent.User, error) {
	u, err := s.accounts.SignUpVerified(ctx, name, email, passwordHash, surveyDue)
	switch {
	case errors.Is(err, accounts.ErrSignupClosed):
		return nil, signup.ErrClosed
	case db.IsUniqueViolation(err):
		return nil, signup.ErrTaken
	}
	return u, err
}

// signUpMail is the mails of a Sign-up: the account mails and the welcome.
type signUpMail struct {
	SystemMailEnqueuer
	WelcomeEnqueuer
}

// newSignUp builds the signup module over the site's adapter (ADR 0030, 0031).
func (h *Handlers) newSignUp() *signup.Module {
	return signup.New(signup.Config{
		Store:  signUpStore{h.accounts},
		Budget: h.attempts.For(accounts.KindSignUp),
		Mail:   signUpMail{h.sysmail, h.welcome},
		Mint: func(u *ent.User, browserHash string) (string, error) {
			return h.tokens.Mint(authtoken.PurposeEmailVerify, u.ID, u.PasswordHash, verifyTokenTTL, auth.VerifyClaims(u.Email, browserHash))
		},
		RequireVerification: h.requireVerification,
		SurveyDue:           h.signupSurvey.Enabled,
	})
}

// signupClosed is the 403 answer to a Sign-up the Instance's sign-up rule refuses
// (ADR 0060); accepting an Invitation still works.
func signupClosed() siteapi.SiteAuthSignUpRes {
	v := siteapi.SiteAuthSignUpForbidden(siteapi.ProblemDetails{
		Code:   siteapi.NewOptProblemCode(siteapi.ProblemCodeSignupClosed),
		Status: siteapi.NewOptInt32(http.StatusForbidden),
		Title:  siteapi.NewOptString(http.StatusText(http.StatusForbidden)),
		Detail: siteapi.NewOptString(i18n.T("errors.signup_closed", nil)),
	})
	return &v
}

// emailTaken is the 409 answer to a Sign-up with an address already in use, on an Instance
// that does not require email verification.
func emailTaken() siteapi.SiteAuthSignUpRes {
	v := siteapi.SiteAuthSignUpConflict(problemWithErrors(
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
	// The same binding as at Sign-up: the link works until the password is replaced
	// or the address verified. It carries the hash of the Sign-up browser cookie the request
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
	claims := auth.VerifyClaims(to, signup.BrowserHash(params.SIGNUPBROWSER.Or("")))
	token, err := h.tokens.Mint(authtoken.PurposeEmailVerify, uid, binding, verifyTokenTTL, claims)
	if err != nil {
		return nil, err
	}
	if err := h.sysmail.EnqueueEmailVerification(ctx, to, token, u != nil && u.EmailVerifiedAt == nil); err != nil {
		mailNotQueued(ctx, "EmailVerification", err)
	}
	return &siteapi.SiteAuthResendVerificationAccepted{}, nil
}
