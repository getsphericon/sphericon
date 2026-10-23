package site

import (
	"context"
	"net/http"
	"strings"

	"github.com/getsphericon/sphericon/ent"
	siteapi "github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/accounts"
	"github.com/getsphericon/sphericon/internal/api/auth"
	"github.com/getsphericon/sphericon/internal/authtoken"
	"github.com/getsphericon/sphericon/internal/credentials"
	"github.com/getsphericon/sphericon/internal/i18n"
	"github.com/getsphericon/sphericon/internal/login"
	"github.com/getsphericon/sphericon/internal/ratelimit"
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

// newPasswordForm is the new password SiteUserUpdateMe sets, checked beyond the contract.
type newPasswordForm struct {
	NewPassword string `json:"newPassword" validate:"password_not_blank,password_length"`
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
		// The rule is checked before the current password is: a refused one costs no hash.
		fieldErrors, err := checkFields(newPasswordForm{NewPassword: newPassword})
		if err != nil {
			return nil, err
		}
		if fieldErrors != nil {
			v := siteapi.SiteUserUpdateMeUnprocessableEntity(fieldsProblem(fieldErrors))
			return &v, nil
		}
		currentPassword, _ := req.CurrentPassword.Get()
		if currentPassword == "" {
			v := siteapi.SiteUserUpdateMeUnprocessableEntity(problemWithErrors(
				http.StatusUnprocessableEntity,
				i18n.T("errors.current_password_required", nil),
				map[string][]string{"currentPassword": {i18n.T("errors.current_password_required", nil)}},
			))
			return &v, nil
		}
		if proved, err := h.proveCurrentPassword(ctx, u, currentPassword); err != nil {
			return nil, err
		} else if !proved {
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
		cookie, err := h.sessions.Issue(u, currentSession(ctx))
		if err != nil {
			return nil, err
		}
		res.SetCookie = siteapi.NewOptString(cookie.String())
	}
	return res, nil
}

// emailChangeForm is what SiteUserEmailChange checks beyond the contract, the address trimmed.
type emailChangeForm struct {
	NewEmail string `json:"newEmail" validate:"required,account_email"`
}

// SiteUserEmailChange requests a change of the login email. It verifies the
// current password, and emails a confirmation link to the NEW address (or, when that
// address is Taken, a notice to its owner: the answer is the same) — the change only
// takes effect once that link is confirmed (SiteAuthConfirmEmailChange). Returns 202.
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
	fieldErrors, err := checkFields(emailChangeForm{NewEmail: newEmail})
	if err != nil {
		return nil, err
	}
	if fieldErrors != nil {
		v := siteapi.SiteUserEmailChangeUnprocessableEntity(fieldsProblem(fieldErrors))
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

	if proved, err := h.proveCurrentPassword(ctx, u, req.CurrentPassword); err != nil {
		return nil, err
	} else if !proved {
		v := siteapi.SiteUserEmailChangeForbidden(problem(http.StatusForbidden, i18n.T("errors.current_password_incorrect", nil)))
		return &v, nil
	}

	// Whether the new address is Taken is never told to the requester (ADR 0031): every
	// request takes a slot of the address's budget, mints a token and queues exactly one
	// mail job to that address. Free, it is the confirm link; Taken, it is a tokenless
	// notice to its owner. A job with no mail due (budget spent) is a no-send one.
	granted, err := h.attempts.Take(ctx, accounts.KindEmailChange, newEmail)
	if err != nil {
		return nil, err
	}
	taken, err := h.accounts.EmailTaken(ctx, newEmail)
	if err != nil {
		return nil, err
	}

	// Bind to the current email so the token is single-use (invalid once swapped);
	// carry the requested new address in the token.
	token, err := h.tokens.Mint(authtoken.PurposeEmailChange, u.ID, u.Email, emailChangeTokenTTL, map[string]string{"new": newEmail})
	if err != nil {
		return nil, err
	}
	if taken {
		_ = h.sysmail.EnqueueAddressTakenNotice(ctx, newEmail, granted)
	} else {
		_ = h.sysmail.EnqueueEmailChangeConfirm(ctx, newEmail, token, granted)
	}

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

// proveCurrentPassword is the profile's proof of the current password: the login module's
// one prove path (throttle first, decoy hash, a failure counted on the Login budget).
// While the address's delay runs it returns the 429 error even for a right password.
func (h *Handlers) proveCurrentPassword(ctx context.Context, u *ent.User, password string) (bool, error) {
	res, err := h.login.Prove(ctx, u.Email, u.PasswordHash, password)
	if err != nil {
		return false, err
	}
	if res.Proof == login.ProofThrottled {
		return false, loginDelayError(ctx, res.Delay)
	}
	return res.Proof == login.ProofHolds, nil
}

// loginDelayError is the 429 rejection of a running Login throttle delay.
func loginDelayError(ctx context.Context, d login.Delay) error {
	return ratelimit.FromContext(ctx).Delay(ctx, ratelimit.PolicyLoginAccount, d.Limit, d.Wait, d.Now)
}
