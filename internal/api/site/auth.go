package site

import (
	"context"
	"strings"

	"github.com/getsphericon/sphericon/ent"
	siteapi "github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/consentrecords"
	"github.com/getsphericon/sphericon/internal/infra/db"
	"github.com/getsphericon/sphericon/internal/infra/throttle"
	"github.com/getsphericon/sphericon/internal/kernel/mailjobs"
	"github.com/getsphericon/sphericon/internal/usecase/signup"
	"github.com/getsphericon/sphericon/internal/usecase/workspacecreation"
	"github.com/getsphericon/sphericon/internal/users"
)

// SiteAuthSignUp is the Sign-up (ADR 0030): the signup module decides, and the answer
// carries its outcome and, when a verification link follows, the Sign-up browser's cookie
// (whatever the address, ADR 0031). No session starts and no Workspace is made here; they
// follow the proof.
func (h *Handlers) SiteAuthSignUp(ctx context.Context, req *siteapi.SiteSignUpInput) (*siteapi.SiteAuthSignUpAccepted, error) {
	res, err := h.signup.SignUp(ctx, inputs.SignUp(req))
	if err != nil {
		return nil, err
	}
	accepted := &siteapi.SiteAuthSignUpAccepted{}
	if res.Browser != "" {
		accepted.SetCookie = siteapi.NewOptString(h.sessions.SignUpBrowser(res.Browser, users.VerificationLinkTTL).String())
	}
	return accepted, nil
}

// signUpStore is the site's side of the Sign-up's seam: Users through the users module, and a
// Verified User with its first Workspace through the Workspace creation.
type signUpStore struct {
	users    *users.Module
	creation *workspacecreation.UseCase
}

func (s signUpStore) Lookup(ctx context.Context, email string) (*ent.User, bool, error) {
	u, err := s.users.UserByEmail(ctx, email)
	if ent.IsNotFound(err) {
		return nil, false, nil
	}
	return u, err == nil, err
}

func (s signUpStore) CreateUnverified(ctx context.Context, n users.NewUser) (*ent.User, error) {
	u, err := s.users.CreateUnverified(ctx, n)
	if db.IsUniqueViolation(err) {
		return nil, signup.ErrTaken
	}
	return u, err
}

func (s signUpStore) ReplaceUnverifiedPassword(ctx context.Context, id int64, passwordHash string, consents consentrecords.Consents) (*ent.User, error) {
	return s.users.ReplaceUnverifiedPassword(ctx, id, passwordHash, consents)
}

func (s signUpStore) CreateVerified(ctx context.Context, n users.NewUser) (*ent.User, error) {
	u, err := s.creation.SignUpVerified(ctx, n)
	if db.IsUniqueViolation(err) {
		return nil, signup.ErrTaken
	}
	return u, err
}

// newSignUp builds the Sign-up use case over the site's adapter (ADR 0030, 0031).
func (h *Handlers) newSignUp() *signup.UseCase {
	return signup.New(signup.Config{
		Store:               signUpStore{users: h.users, creation: h.workspaceCreation},
		Consents:            h.consentRecords,
		Budget:              h.attempts.For(throttle.KindSignUp),
		Passwords:           h.passwords,
		Mail:                h.mail,
		Mint:                h.links.VerificationLink,
		RequireVerification: h.requireVerification,
		SurveyDue:           h.signupSurvey.Enabled,
	})
}

// SiteAuthResendVerification sends the verification link again to an Unverified User
// (ADR 0030). It answers 202 the same way for an unknown, an already verified and an
// unverified address, and every request does the same work (a token is minted, a mail
// job is queued); only an Unverified User's job delivers, the others' is a no-send one,
// so neither the answer nor its timing says whether the address has an account. The
// per-IP cap answers 429 in the rate limit middleware, before this runs.
func (h *Handlers) SiteAuthResendVerification(ctx context.Context, req *siteapi.SiteResendVerificationInput, params siteapi.SiteAuthResendVerificationParams) error {
	email := strings.TrimSpace(string(req.Email))
	if email == "" {
		return nil
	}
	u, err := h.users.UserByEmail(ctx, email)
	if err != nil && !ent.IsNotFound(err) {
		return err
	}
	// The same link as at Sign-up: it works until the password is replaced or the address
	// verified. It carries the hash of the Sign-up browser cookie the request holds, none
	// otherwise (then the link asks for a password); every request hashes the same way, so a
	// cookie changes no timing. An unknown address gets a throwaway link over the same work.
	// The cookie is SameSite=Strict: the browser withholds it on the cross-site click from
	// the mail, but sends it on the page's own request that follows (same site), which is
	// the one that carries the token.
	to := email
	if u != nil {
		to = u.Email
	}
	token, err := h.links.VerificationLink(u, users.BrowserHash(params.SIGNUPBROWSER.Or("")))
	if err != nil {
		return err
	}
	if err := h.mail.Enqueue(ctx, mailjobs.EmailVerification(to, token, u != nil && u.EmailVerifiedAt == nil)); err != nil {
		mailNotQueued(ctx, "EmailVerification", err)
	}
	return nil
}
