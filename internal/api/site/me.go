package site

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/getsphericon/sphericon/ent"
	siteapi "github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/accounts"
	"github.com/getsphericon/sphericon/internal/api/auth"
	"github.com/getsphericon/sphericon/internal/authtoken"
	"github.com/getsphericon/sphericon/internal/i18n"
	"github.com/getsphericon/sphericon/internal/login"
	"github.com/getsphericon/sphericon/internal/ratelimit"
	"github.com/getsphericon/sphericon/internal/validation"
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
	return h.userResource(u), nil
}

// userResource is the User's profile with the Sign-up survey due only while the User carries the
// marker and the Instance still asks the survey: a switch turned off or a lapsed license stops
// asking at once, for a User marked earlier too.
func (h *Handlers) userResource(u *ent.User) *siteapi.SiteUserResource {
	res := mapper.UserToResource(u)
	res.SignupSurveyDue = u.SignupSurveyDueAt != nil && h.signupSurvey.Enabled()
	return res
}

// SiteUserUpdateMe changes the authenticated User's name and/or password through accounts,
// which holds the profile rules. The email is the login identity and is not editable here. A
// password change ended every session; the acting one continues under a fresh cookie.
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
	change := accounts.ProfileChange{NewPassword: req.NewPassword.Or(""), CurrentPassword: req.CurrentPassword.Or("")}
	if name, ok := req.Name.Get(); ok {
		change.Name = &name
	}
	changed, err := h.accounts.ChangeProfile(ctx, u, change, h.proveCurrentPassword)
	if fields, ok := errors.AsType[*validation.FieldsError](err); ok {
		v := siteapi.SiteUserUpdateMeUnprocessableEntity(fieldsProblem(fields.Fields))
		return &v, nil
	}
	if errors.Is(err, accounts.ErrCurrentPasswordIncorrect) {
		v := siteapi.SiteUserUpdateMeForbidden(problem(http.StatusForbidden, validation.Message(err)))
		return &v, nil
	}
	if err != nil {
		return nil, err
	}
	res := &siteapi.SiteUserResourceHeaders{Response: *h.userResource(changed.User)}
	if changed.PasswordChanged {
		cookie, err := h.sessions.Issue(changed.User, currentSession(ctx))
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
			map[string][]string{fieldNewEmail: {i18n.T("errors.new_email_must_differ", nil)}},
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
		if err := h.sysmail.EnqueueAddressTakenNotice(ctx, newEmail, granted); err != nil {
			mailNotQueued(ctx, "AddressTakenNotice", err)
		}
	} else {
		if err := h.sysmail.EnqueueEmailChangeConfirm(ctx, newEmail, token, granted); err != nil {
			mailNotQueued(ctx, "EmailChangeConfirm", err)
		}
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
