package site

import (
	"context"
	"net/http"
	"strings"

	siteapi "github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/api/auth"
	"github.com/getsphericon/sphericon/internal/authtoken"
	"github.com/getsphericon/sphericon/internal/credentials"
	"github.com/getsphericon/sphericon/internal/i18n"
)

// SiteUserGetMe returns the authenticated user's profile. Auth is enforced by
// the security handler, so SiteAuth is always present here.
func (h *Handlers) SiteUserGetMe(ctx context.Context) (*siteapi.SiteUserResource, error) {
	a := auth.GetSiteAuth(ctx)
	if a == nil {
		return nil, auth.ErrUnauthorized
	}
	u, err := h.accounts.User(ctx, a.UserID)
	if err != nil {
		return nil, err
	}
	return mapper.UserToResource(u), nil
}

// SiteUserUpdateMe updates the authenticated user's name and/or password. Email
// is the login identity and is not editable here. Changing the password
// requires the correct current password.
func (h *Handlers) SiteUserUpdateMe(ctx context.Context, req *siteapi.SiteUpdateMeInput) (siteapi.SiteUserUpdateMeRes, error) {
	a := auth.GetSiteAuth(ctx)
	if a == nil {
		v := siteapi.SiteUserUpdateMeForbidden(problem(http.StatusForbidden, "unauthorized"))
		return &v, nil
	}
	u, err := h.accounts.User(ctx, a.UserID)
	if err != nil {
		return nil, err
	}

	var newName, newHash *string

	if name, ok := req.Name.Get(); ok {
		name = strings.TrimSpace(name)
		if name == "" {
			v := siteapi.SiteUserUpdateMeUnprocessableEntity(problemWithErrors(
				http.StatusUnprocessableEntity,
				i18n.T("errors.name_empty", nil),
				map[string][]string{"name": {i18n.T("errors.name_empty", nil)}},
			))
			return &v, nil
		}
		newName = &name
	}

	if newPassword, ok := req.NewPassword.Get(); ok && newPassword != "" {
		currentPassword, _ := req.CurrentPassword.Get()
		if currentPassword == "" {
			v := siteapi.SiteUserUpdateMeUnprocessableEntity(problemWithErrors(
				http.StatusUnprocessableEntity,
				i18n.T("errors.current_password_required", nil),
				map[string][]string{"currentPassword": {i18n.T("errors.current_password_required", nil)}},
			))
			return &v, nil
		}
		// Verify the current password the same way the direct login provider does.
		if u.PasswordHash == "" || !credentials.VerifyPassword(u.PasswordHash, currentPassword) {
			v := siteapi.SiteUserUpdateMeForbidden(problem(http.StatusForbidden, i18n.T("errors.current_password_incorrect", nil)))
			return &v, nil
		}
		hash, err := credentials.HashPassword(newPassword)
		if err != nil {
			return nil, err
		}
		newHash = &hash
	}

	if newName != nil || newHash != nil {
		u, err = h.accounts.UpdateProfile(ctx, u.ID, newName, newHash)
		if err != nil {
			return nil, err
		}
	}
	res := &siteapi.SiteUserResourceHeaders{Response: *mapper.UserToResource(u)}
	// The password change ended every session; the acting one continues under a
	// fresh token stamped with the new epoch.
	if newHash != nil {
		cookie, err := h.sessions.Issue(u)
		if err != nil {
			return nil, err
		}
		res.SetCookie = siteapi.NewOptString(cookie.String())
	}
	return res, nil
}

// SiteUserEmailChange requests a change of the login email. It verifies the
// current password, rejects an address already in use, and emails a
// confirmation link to the NEW address — the change only takes effect once that
// link is confirmed (SiteAuthConfirmEmailChange). Returns 202.
func (h *Handlers) SiteUserEmailChange(ctx context.Context, req *siteapi.SiteEmailChangeInput) (siteapi.SiteUserEmailChangeRes, error) {
	a := auth.GetSiteAuth(ctx)
	if a == nil {
		v := siteapi.SiteUserEmailChangeForbidden(problem(http.StatusForbidden, "unauthorized"))
		return &v, nil
	}
	u, err := h.accounts.User(ctx, a.UserID)
	if err != nil {
		return nil, err
	}

	newEmail := strings.TrimSpace(string(req.NewEmail))
	if newEmail == "" {
		v := siteapi.SiteUserEmailChangeUnprocessableEntity(problemWithErrors(
			http.StatusUnprocessableEntity,
			i18n.T("errors.new_email_required", nil),
			map[string][]string{"newEmail": {i18n.T("errors.new_email_required", nil)}},
		))
		return &v, nil
	}
	if strings.EqualFold(newEmail, u.Email) {
		v := siteapi.SiteUserEmailChangeUnprocessableEntity(problemWithErrors(
			http.StatusUnprocessableEntity,
			i18n.T("errors.new_email_must_differ", nil),
			map[string][]string{"newEmail": {i18n.T("errors.new_email_must_differ", nil)}},
		))
		return &v, nil
	}

	if u.PasswordHash == "" || !credentials.VerifyPassword(u.PasswordHash, req.CurrentPassword) {
		v := siteapi.SiteUserEmailChangeForbidden(problem(http.StatusForbidden, i18n.T("errors.current_password_incorrect", nil)))
		return &v, nil
	}

	// Reject an address already taken. The confirm step re-checks under the unique
	// index, so this is an early, friendly 409 rather than the sole guard.
	taken, err := h.accounts.EmailTaken(ctx, newEmail)
	if err != nil {
		return nil, err
	}
	if taken {
		v := siteapi.SiteUserEmailChangeConflict(problem(http.StatusConflict, i18n.T("errors.email_in_use", nil)))
		return &v, nil
	}

	// Bind to the current email so the token is single-use (invalid once swapped);
	// carry the requested new address in the token.
	token, err := h.tokens.Mint(authtoken.PurposeEmailChange, u.ID, u.Email, emailChangeTokenTTL, map[string]string{"new": newEmail})
	if err != nil {
		return nil, err
	}
	_ = h.sysmail.EnqueueEmailChangeConfirm(ctx, newEmail, token)

	return &siteapi.SiteUserEmailChangeAccepted{}, nil
}

// SiteUserSignOutEverywhere ends every session of the authenticated user, on
// every device, and clears the acting browser's cookie: no session survives.
func (h *Handlers) SiteUserSignOutEverywhere(ctx context.Context) (*siteapi.SiteUserSignOutEverywhereNoContent, error) {
	a := auth.GetSiteAuth(ctx)
	if a == nil {
		return nil, auth.ErrUnauthorized
	}
	if err := h.accounts.EndSessions(ctx, a.UserID); err != nil {
		return nil, err
	}
	return &siteapi.SiteUserSignOutEverywhereNoContent{SetCookie: h.sessions.Cleared().String()}, nil
}
