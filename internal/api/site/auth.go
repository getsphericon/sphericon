package site

import (
	"context"
	"strings"

	"github.com/getsphericon/sphericon/ent"
	siteapi "github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/accounts"
	"github.com/getsphericon/sphericon/internal/consentrecords"
	"github.com/getsphericon/sphericon/internal/db"
	"github.com/getsphericon/sphericon/internal/signup"
)

// SiteAuthSignUp is the Sign-up (ADR 0030): the signup module decides, and the answer
// carries its outcome and, when a verification link follows, the Sign-up browser's cookie
// (whatever the address, ADR 0031). No session starts and no Workspace is made here; they
// follow the proof.
func (h *Handlers) SiteAuthSignUp(ctx context.Context, req *siteapi.SiteSignUpInput) (siteapi.SiteAuthSignUpRes, error) {
	res, err := h.signup.SignUp(ctx, inputs.SignUp(req))
	if err != nil {
		return nil, err
	}
	accepted := &siteapi.SiteAuthSignUpAccepted{}
	if res.Browser != "" {
		accepted.SetCookie = siteapi.NewOptString(h.sessions.SignUpBrowser(res.Browser, accounts.VerificationLinkTTL).String())
	}
	return accepted, nil
}

// signUpStore is the site's side of the signup module's seam: Users through accounts.
type signUpStore struct{ accounts *accounts.Accounts }

func (s signUpStore) Lookup(ctx context.Context, email string) (*ent.User, bool, error) {
	u, err := s.accounts.UserByEmail(ctx, email)
	if ent.IsNotFound(err) {
		return nil, false, nil
	}
	return u, err == nil, err
}

func (s signUpStore) CreateUnverified(ctx context.Context, n signup.NewUser) (*ent.User, error) {
	u, err := s.accounts.CreateUnverified(ctx, n)
	if db.IsUniqueViolation(err) {
		return nil, signup.ErrTaken
	}
	return u, err
}

func (s signUpStore) ReplaceUnverifiedPassword(ctx context.Context, id int64, passwordHash string, consents consentrecords.Consents) (*ent.User, error) {
	return s.accounts.ReplaceUnverifiedPassword(ctx, id, passwordHash, consents)
}

func (s signUpStore) CreateVerified(ctx context.Context, n signup.NewUser) (*ent.User, error) {
	u, err := s.accounts.SignUpVerified(ctx, n)
	if db.IsUniqueViolation(err) {
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
		Store:               signUpStore{h.accounts},
		Consents:            h.consentRecords,
		Budget:              h.attempts.For(accounts.KindSignUp),
		Passwords:           h.passwords,
		Mail:                signUpMail{h.sysmail, h.welcome},
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
func (h *Handlers) SiteAuthResendVerification(ctx context.Context, req *siteapi.SiteResendVerificationInput, params siteapi.SiteAuthResendVerificationParams) (siteapi.SiteAuthResendVerificationRes, error) {
	email := strings.TrimSpace(string(req.Email))
	if email == "" {
		return &siteapi.SiteAuthResendVerificationAccepted{}, nil
	}
	u, err := h.accounts.UserByEmail(ctx, email)
	if err != nil && !ent.IsNotFound(err) {
		return nil, err
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
	token, err := h.links.VerificationLink(u, signup.BrowserHash(params.SIGNUPBROWSER.Or("")))
	if err != nil {
		return nil, err
	}
	if err := h.sysmail.EnqueueEmailVerification(ctx, to, token, u != nil && u.EmailVerifiedAt == nil); err != nil {
		mailNotQueued(ctx, "EmailVerification", err)
	}
	return &siteapi.SiteAuthResendVerificationAccepted{}, nil
}
