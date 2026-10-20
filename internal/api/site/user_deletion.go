package site

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/samber/lo"

	"github.com/getsphericon/sphericon/ent"
	siteapi "github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/accounts"
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
	userID, err := sessionUserID(ctx)
	if err != nil {
		return nil, err
	}
	u, err := h.accounts.User(ctx, userID)
	if err != nil {
		return nil, err
	}
	if !strings.EqualFold(strings.TrimSpace(string(req.Email)), u.Email) {
		msg := i18n.T("errors.delete_user_email_mismatch", nil)
		v := deleteUserProblem(msg, map[string][]string{"email": {msg}}, nil)
		return &v, nil
	}
	proved, err := h.proveCurrentPassword(ctx, u, req.CurrentPassword)
	if err != nil {
		return nil, err
	}
	if !proved {
		v := passwordIncorrectProblem()
		return &v, nil
	}

	confirm := func(ctx context.Context, tx *ent.Client, pub events.Publisher, locked *ent.User) error {
		if !secondfactor.Active(locked) {
			return nil
		}
		_, err := h.secondFactor.VerifyIn(ctx, tx, pub, locked, req.Code.Or(""))
		return err
	}
	deleted, err := h.accounts.DeleteUser(ctx, userID, confirm)
	var sole *accounts.SoleOwnerError
	switch {
	case errors.As(err, &sole):
		msg := i18n.T("errors.delete_user_only_owner", nil)
		listed := lo.Map(sole.Workspaces, func(w *ent.Workspace, _ int) siteapi.SiteSoleOwnedWorkspace {
			return siteapi.SiteSoleOwnedWorkspace{ID: siteapi.WorkspaceId(w.PublicID), Name: w.Name}
		})
		v := deleteUserProblem(msg, nil, listed)
		return &v, nil
	case errors.Is(err, secondfactor.ErrInvalidCode):
		// The wrong code is counted like a wrong password (ADR 0025).
		if err := h.login.CountFailure(ctx, u.Email); err != nil {
			return nil, err
		}
		v := problem(http.StatusForbidden, i18n.T("errors.second_factor_code_invalid", nil))
		return &v, nil
	case err != nil:
		return nil, err
	}

	// The deletion is committed and cannot be undone: a mail that fails to queue does not undo it.
	_ = h.sysmail.EnqueueUserDeletedNotice(ctx, deleted.Email)
	return &siteapi.SiteUserDeleteUserNoContent{SetCookie: h.sessions.Cleared().String()}, nil
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
