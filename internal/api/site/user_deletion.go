package site

import (
	"context"
	"errors"

	siteapi "github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/domain/identity/secondfactors"
	"github.com/getsphericon/sphericon/internal/domain/identity/users"
)

// SiteUserDeleteUser deletes the caller's own User at once (User deletion, ADR 0050), after
// the typed email, the password (through the login module's one prove path, so a wrong one
// is counted on the Login throttle) and, with a Second factor, a current code. The
// accounts operation does the rest in one transaction; this handler authenticates and proves.
func (h *Handlers) SiteUserDeleteUser(ctx context.Context, req *siteapi.SiteDeleteUserInput) (*siteapi.SiteUserDeleteUserNoContent, error) {
	userID, err := sessionUserID(ctx)
	if err != nil {
		return nil, err
	}
	u, err := h.users.DeletionSubject(ctx, userID, string(req.Email))
	if err != nil {
		return nil, err
	}
	hadPassword := u.PasswordHash != ""
	if hadPassword {
		if err := h.proveCurrentPassword(ctx, u, req.CurrentPassword.Or("")); err != nil {
			return nil, err
		}
	} else {
		// A User with no password proves the mailbox instead, and nothing in that proof
		// feeds the Login throttle: consult it here, before the link or any code is
		// checked, so the code cannot be guessed past the budget (ADR 0025).
		delay, throttled, err := h.login.Throttled(ctx, u.Email)
		if err != nil {
			return nil, err
		}
		if throttled {
			return nil, loginDelayError(ctx, delay)
		}
		if err := h.links.CheckDeletionLink(req.Token.Or(""), u); err != nil {
			return nil, err
		}
	}

	_, err = h.users.DeleteUser(ctx, u.ID, &users.DeleteProof{HadPassword: hadPassword, Confirm: h.secondFactor.DeletionCode(req.Code.Or(""))})
	if errors.Is(err, secondfactors.ErrDeletionCodeInvalid) {
		// The wrong code is counted like a wrong password (ADR 0025).
		if cerr := h.login.CountFailure(ctx, u.Email); cerr != nil {
			return nil, cerr
		}
	}
	if err != nil {
		return nil, err
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
