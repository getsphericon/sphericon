package site

import (
	"context"

	siteapi "github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/domain/identity/users"
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

// SiteAuthResendVerification sends the verification link again to an Unverified User (ADR 0030,
// users.Requests.Verification). It answers 202 the same way for an unknown, an already verified
// and an unverified address, so neither the answer nor its timing says whether the address has an
// account. The per-IP cap answers 429 in the rate limit middleware, before this runs. The Sign-up
// browser cookie is SameSite=Strict: the browser withholds it on the cross-site click from the
// mail, but sends it on the page's own request that follows (same site), which carries the token.
func (h *Handlers) SiteAuthResendVerification(ctx context.Context, req *siteapi.SiteResendVerificationInput, params siteapi.SiteAuthResendVerificationParams) error {
	return h.requests.Verification(ctx, string(req.Email), params.SIGNUPBROWSER.Or(""))
}
