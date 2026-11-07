package site

import (
	"context"
	"strings"

	"github.com/getsphericon/sphericon/ent"
	siteapi "github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/domain/identity/users"
	"github.com/getsphericon/sphericon/internal/infra/events"
)

// SiteAuthSignUp is the Sign-up (ADR 0030): the Workspace creation decides, and the answer
// carries its outcome and, when a verification link follows, the Sign-up browser's cookie
// (whatever the address, ADR 0031). No session starts and no Workspace is made here; they
// follow the proof.
func (h *Handlers) SiteAuthSignUp(ctx context.Context, req *siteapi.SiteSignUpInput) (*siteapi.SiteAuthSignUpAccepted, error) {
	res, err := h.signUp.SignUp(ctx, inputs.SignUp(req))
	if err != nil {
		return nil, err
	}
	accepted := &siteapi.SiteAuthSignUpAccepted{}
	if res.Browser != "" {
		accepted.SetCookie = siteapi.NewOptString(h.sessions.SignUpBrowser(res.Browser, users.VerificationLinkTTL).String())
	}
	return accepted, nil
}

// SiteAuthResendVerification sends the verification link again to an Unverified User
// (ADR 0030). It answers 202 the same way for an unknown, an already verified and an
// unverified address, and every request does the same work (a lookup, a published request);
// only an Unverified User's request is delivered, so neither the answer nor its timing says
// whether the address has an account. The per-IP cap answers 429 in the rate limit
// middleware, before this runs.
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
	// verified. It trusts the Sign-up browser cookie the request holds, none otherwise (then the
	// link asks for a password); every request hashes the same way, so a cookie changes no
	// timing. The cookie is SameSite=Strict: the browser withholds it on the cross-site click
	// from the mail, but sends it on the page's own request that follows (same site), which is
	// the one that carries the token.
	request := &events.LinkRequested{
		Action: events.NameUserVerificationRequested, Email: email,
		BrowserHash: users.BrowserHash(params.SIGNUPBROWSER.Or("")),
	}
	if u != nil {
		request.UserID, request.Email, request.Deliver = u.ID, u.Email, u.EmailVerifiedAt == nil
	}
	h.publishRequest(ctx, request)
	return nil
}
