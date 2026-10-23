package site

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/samber/lo"

	"github.com/getsphericon/sphericon/ent"
	siteapi "github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/accounts"
	"github.com/getsphericon/sphericon/internal/authtoken"
	"github.com/getsphericon/sphericon/internal/events"
	"github.com/getsphericon/sphericon/internal/i18n"
	"github.com/getsphericon/sphericon/internal/secondfactor"
)

// SiteUserDeleteUser deletes the caller's own User at once (User deletion, ADR 0050), after
// the typed email, the password (through the login module's one prove path, so a wrong one
// is counted on the Login throttle) and, with a Second factor, a current code. The
// accounts operation does the rest in one transaction; this handler authenticates, proves
// and maps the outcomes.
func (h *Handlers) SiteUserDeleteUser(ctx context.Context, req *siteapi.SiteDeleteUserInput) (siteapi.SiteUserDeleteUserRes, error) {
	u, mismatch, err := h.deletionSubject(ctx, string(req.Email))
	if err != nil {
		return nil, err
	}
	if mismatch != nil {
		return mismatch, nil
	}
	hadPassword := u.PasswordHash != ""
	if hadPassword {
		proved, err := h.proveCurrentPassword(ctx, u, req.CurrentPassword.Or(""))
		if err != nil {
			return nil, err
		}
		if !proved {
			v := siteapi.SiteUserDeleteUserForbidden(passwordIncorrectProblem())
			return &v, nil
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
		if !h.holdsDeletionLink(req.Token.Or(""), u) {
			// The link is bound to this User, so an expired, used or foreign one (a new
			// User on the same address) is dead.
			v := siteapi.SiteUserDeleteUserBadRequest(problem(http.StatusBadRequest, i18n.T("errors.confirm_link_invalid", nil)))
			return &v, nil
		}
	}

	confirm := func(ctx context.Context, tx *ent.Client, pub events.Publisher, locked *ent.User) error {
		// The proof above was read before the transaction: a password set or removed since
		// then makes it the wrong proof for this User.
		if (locked.PasswordHash != "") != hadPassword {
			return errProofStale
		}
		if !secondfactor.Active(locked) {
			return nil
		}
		_, err := h.secondFactor.VerifyIn(ctx, tx, pub, locked, req.Code.Or(""))
		return err
	}
	deleted, err := h.accounts.DeleteUser(ctx, u.ID, confirm)
	var sole *accounts.SoleOwnerError
	switch {
	case errors.As(err, &sole):
		msg := i18n.T("errors.delete_user_only_owner", nil)
		listed := lo.Map(sole.Workspaces, func(w *ent.Workspace, _ int) siteapi.SiteSoleOwnedWorkspace {
			return siteapi.SiteSoleOwnedWorkspace{ID: siteapi.WorkspaceId(w.PublicID), Name: w.Name}
		})
		v := deleteUserProblem(msg, nil, listed)
		return &v, nil
	case errors.Is(err, errProofStale):
		v := deleteUserProblem(i18n.T("errors.delete_user_proof_stale", nil), nil, nil)
		return &v, nil
	case errors.Is(err, secondfactor.ErrInvalidCode):
		// The wrong code is counted like a wrong password (ADR 0025).
		if err := h.login.CountFailure(ctx, u.Email); err != nil {
			return nil, err
		}
		v := siteapi.SiteUserDeleteUserForbidden(problem(http.StatusForbidden, i18n.T("errors.second_factor_code_invalid", nil)))
		return &v, nil
	case err != nil:
		return nil, err
	}

	// The deletion is committed and cannot be undone: a mail that fails to queue does not undo it.
	_ = h.sysmail.EnqueueUserDeletedNotice(ctx, deleted.Email)
	return &siteapi.SiteUserDeleteUserNoContent{SetCookie: h.sessions.Cleared().String()}, nil
}

// SiteUserDeleteUserLink mails the one-time deletion link to a User with no password (ADR
// 0050): the typed email must be the User's, and the link goes to that Verified address. The
// token is bound to the User and their session epoch, not to the address, and works once (the
// deletion removes the User it names).
func (h *Handlers) SiteUserDeleteUserLink(ctx context.Context, req *siteapi.SiteDeleteUserLinkInput) (siteapi.SiteUserDeleteUserLinkRes, error) {
	u, mismatch, err := h.deletionSubject(ctx, string(req.Email))
	if err != nil {
		return nil, err
	}
	if mismatch != nil {
		return mismatch, nil
	}
	if u.PasswordHash != "" {
		v := deleteUserProblem(i18n.T("errors.delete_user_use_password", nil), nil, nil)
		return &v, nil
	}
	token, err := h.tokens.Mint(authtoken.PurposeUserDeletion, u.ID, deletionLinkBinding(u), deleteUserLinkTTL, nil)
	if err != nil {
		return nil, err
	}
	// Like the email-change link, every request takes a slot of the address's budget; a spent
	// budget queues a mail job that sends nothing.
	granted, err := h.attempts.Take(ctx, accounts.KindEmailChange, u.Email)
	if err != nil {
		return nil, err
	}
	_ = h.sysmail.EnqueueUserDeletionLink(ctx, u.Email, token, granted)
	return &siteapi.SiteUserDeleteUserLinkAccepted{}, nil
}

// errProofStale is the refusal of a deletion whose proof no longer fits the User: the User's
// password was set or removed between the proof and the transaction that locked the row.
var errProofStale = errors.New("the deletion proof no longer fits the user")

// deletionSubject is the start of both deletion operations: the User of the session and the
// check that the typed email is theirs. A mismatch is the 422 to answer with, nil when the
// email is the User's own.
func (h *Handlers) deletionSubject(ctx context.Context, typedEmail string) (*ent.User, *siteapi.SiteDeleteUserProblemDetails, error) {
	userID, err := sessionUserID(ctx)
	if err != nil {
		return nil, nil, err
	}
	u, err := h.accounts.User(ctx, userID)
	if err != nil {
		return nil, nil, err
	}
	if !strings.EqualFold(strings.TrimSpace(typedEmail), u.Email) {
		msg := i18n.T("errors.delete_user_email_mismatch", nil)
		v := deleteUserProblem(msg, map[string][]string{"email": {msg}}, nil)
		return u, &v, nil
	}
	return u, nil, nil
}

// deleteUserLinkTTL is how long the mailed deletion link lives.
const deleteUserLinkTTL = time.Hour

// deletionLinkBinding keys the deletion token to the User's session epoch, so signing out
// everywhere (or any other epoch bump) kills a link already mailed.
func deletionLinkBinding(u *ent.User) string { return strconv.FormatInt(u.SessionEpoch, 10) }

// holdsDeletionLink reports whether raw is a live deletion link issued to u.
func (h *Handlers) holdsDeletionLink(raw string, u *ent.User) bool {
	uid, _, err := h.tokens.Parse(raw, authtoken.PurposeUserDeletion, func(id int64) (string, error) {
		if id != u.ID {
			return "", errors.New("deletion link of another user")
		}
		return deletionLinkBinding(u), nil
	})
	return err == nil && uid == u.ID
}

// deleteUserProblem is the 422 body of a refused User deletion: the field errors of a
// mismatched email, or the Workspaces the User solely owns.
func deleteUserProblem(detail string, errs map[string][]string, workspaces []siteapi.SiteSoleOwnedWorkspace) siteapi.SiteDeleteUserProblemDetails {
	p := siteapi.SiteDeleteUserProblemDetails{
		Status:     siteapi.NewOptInt32(http.StatusUnprocessableEntity),
		Title:      siteapi.NewOptString(http.StatusText(http.StatusUnprocessableEntity)),
		Detail:     siteapi.NewOptString(detail),
		Workspaces: workspaces,
	}
	if errs != nil {
		p.Errors = siteapi.NewOptSiteDeleteUserProblemDetailsErrors(siteapi.SiteDeleteUserProblemDetailsErrors(errs))
	}
	return p
}
