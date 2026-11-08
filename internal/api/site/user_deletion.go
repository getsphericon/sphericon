package site

import (
	"context"

	siteapi "github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/usecase/userdeletion"
)

// SiteUserDeleteUser deletes the caller's own User at once (User deletion, ADR 0050): the use
// case checks the typed email, the proof and the Second factor code, and deletes in one
// transaction. The handler answers a running Login throttle delay with the 429 and clears the
// session cookie.
func (h *Handlers) SiteUserDeleteUser(ctx context.Context, req *siteapi.SiteDeleteUserInput) (*siteapi.SiteUserDeleteUserNoContent, error) {
	userID, err := sessionUserID(ctx)
	if err != nil {
		return nil, err
	}
	res, err := h.userDeletion.Delete(ctx, userdeletion.Request{
		UserID:   userID,
		Email:    string(req.Email),
		Password: req.CurrentPassword.Or(""),
		Token:    req.Token.Or(""),
		Code:     req.Code.Or(""),
	})
	if err != nil {
		return nil, err
	}
	if res.Outcome == userdeletion.Throttled {
		return nil, loginDelayError(ctx, res.Delay)
	}
	return &siteapi.SiteUserDeleteUserNoContent{SetCookie: h.sessions.Cleared().String()}, nil
}

// SiteUserDeleteUserLink mails the one-time deletion link to a User with no password (ADR
// 0050): the typed email must be the User's, and the link goes to that Verified address.
func (h *Handlers) SiteUserDeleteUserLink(ctx context.Context, req *siteapi.SiteDeleteUserLinkInput) error {
	userID, err := sessionUserID(ctx)
	if err != nil {
		return err
	}
	return h.requests.DeletionLink(ctx, userID, string(req.Email))
}
