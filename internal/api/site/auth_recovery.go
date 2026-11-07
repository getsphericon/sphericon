package site

import (
	"context"
	"strings"

	"github.com/getsphericon/sphericon/ent"
	siteapi "github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/api/auth"
	"github.com/getsphericon/sphericon/internal/infra/events"
	"github.com/getsphericon/sphericon/internal/infra/throttle"
)

// SiteAuthForgotPassword asks for a reset link to be mailed. It answers 202 whether or not the
// address matches an account and whether or not the per-address budget is spent, so the endpoint
// cannot be used to enumerate the addresses that hold a User (the per-IP cap answers 429 in the
// rate limit middleware, before this runs). Every request does the same work: take a slot of the
// address's budget in one atomic step, look the address up and publish the request. Only a known
// address that was granted a slot gets its link, minted when it is mailed and bound to the
// password as it is then, so it works once.
func (h *Handlers) SiteAuthForgotPassword(ctx context.Context, req *siteapi.SiteForgotPasswordInput) error {
	email := strings.TrimSpace(string(req.Email))
	if email == "" {
		return nil
	}
	// Unknown addresses spend their budget exactly like known ones.
	granted, err := h.attempts.Take(ctx, throttle.KindPasswordReset, email)
	if err != nil {
		return err
	}
	u, err := h.users.UserByEmail(ctx, email)
	if err != nil && !ent.IsNotFound(err) {
		return err
	}
	request := &events.LinkRequested{Action: events.NameUserPasswordResetRequested, Email: email}
	if u != nil {
		request.UserID, request.Email, request.Deliver = u.ID, u.Email, granted
	}
	h.publishRequest(ctx, request)
	return nil
}

// SiteAuthResetPassword sets a new password from a reset link (users.Links, through the use case
// signup, which completes the Sign-up of an Unverified User and then welcomes them).
func (h *Handlers) SiteAuthResetPassword(ctx context.Context, req *siteapi.SiteResetPasswordInput) error {
	_, err := h.signUp.ResetPasswordLink(ctx, req.Token, req.Password)
	return err
}

// SiteAuthVerifyEmail completes a Sign-up from its link (ADR 0030, the use case signup): the
// token proves the address, so it stamps the Verified email, ends every earlier session,
// creates the User's first Workspace and welcomes the User, then starts the session through
// the login issuer. The password stored at Sign-up stays only when the request carries the
// Sign-up browser cookie the token was minted for; any other browser must send a password,
// which replaces it.
func (h *Handlers) SiteAuthVerifyEmail(ctx context.Context, req *siteapi.SiteVerifyEmailInput, params siteapi.SiteAuthVerifyEmailParams) (*siteapi.SiteVerifyEmailResultHeaders, error) {
	link := inputs.VerifyEmailLink(req)
	link.Browser = params.SIGNUPBROWSER.Or("")
	done, err := h.signUp.VerifyEmailLink(ctx, link)
	if err != nil {
		return nil, err
	}
	verified, landing := done.User, done.Landing
	// The session starts after the epoch bump VerifyEmailLink made, so it is the only one.
	cookie, err := h.sessions.Issue(verified, auth.Session{Method: auth.MethodEmail})
	if err != nil {
		return nil, err
	}
	return &siteapi.SiteVerifyEmailResultHeaders{
		SetCookie: cookie.String(),
		Response:  mapper.VerifyEmailResultToResource(landing),
	}, nil
}

// SiteAuthConfirmEmailChange applies a requested email change once the new
// address is confirmed via the token sent to it. Public: the link may be opened
// from a browser without a session. The change ends every session of the User
// (ADR 0020); when the confirming request carries a valid session of that same
// User, it continues under a fresh token in the same response. The link alone
// never signs anyone in.
func (h *Handlers) SiteAuthConfirmEmailChange(ctx context.Context, req *siteapi.SiteConfirmEmailChangeInput, params siteapi.SiteAuthConfirmEmailChangeParams) (*siteapi.SiteAuthConfirmEmailChangeOK, error) {
	change, err := h.links.EmailChange(ctx, req.Token)
	if err != nil {
		return nil, err
	}
	uid := change.UserID
	// Checked before the change: the epoch bump ends this session too.
	acting, err := h.holdsSession(ctx, params.JWT.Or(""), uid)
	if err != nil {
		return nil, err
	}
	// A token that records no known method is an invalid session: it is not continued.
	sess, valid := h.sessions.SessionOf(params.JWT.Or(""))
	acting = acting && valid
	if err := h.links.ConfirmEmailChange(ctx, change); err != nil {
		return nil, err
	}
	res := &siteapi.SiteAuthConfirmEmailChangeOK{}
	if acting {
		u, err := h.users.User(ctx, uid)
		if err != nil {
			return nil, err
		}
		cookie, err := h.sessions.Issue(u, sess)
		if err != nil {
			return nil, err
		}
		res.SetCookie = siteapi.NewOptString(cookie.String())
	}
	return res, nil
}

// holdsSession reports whether a raw session token is a current session of the
// User uid: it verifies and carries uid and that User's present epoch.
func (h *Handlers) holdsSession(ctx context.Context, raw string, uid int64) (bool, error) {
	id, epoch, ok := h.sessions.Holder(raw)
	if !ok || id != uid {
		return false, nil
	}
	u, err := h.users.User(ctx, uid)
	if err != nil {
		return false, err
	}
	return u.SessionEpoch == epoch, nil
}
