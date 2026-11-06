package problem

import (
	"errors"
	"net/http"

	"github.com/samber/lo"

	siteapi "github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/domain/identity/users"
	"github.com/getsphericon/sphericon/internal/domain/identity/workspaces"
	"github.com/getsphericon/sphericon/internal/domain/orchestration/inuse"
	"github.com/getsphericon/sphericon/internal/kernel/emailrender"
	"github.com/getsphericon/sphericon/internal/kernel/narrow"
	"github.com/getsphericon/sphericon/internal/kernel/validation"
)

// The refusals every surface answers alike (review rule 15): a handler returns the module's error
// and refusal turns it into the problem, so no handler maps them itself. The core error types know
// nothing of HTTP; this is where they meet a status. A module's sentinel refusal
// (validation.RefusalError) carries its kind, and the kind decides the status and the code: not
// found 404, refused 422, conflict 409, forbidden 403 with code permission_required, unauthorized
// 401, denied 403, unverified 403 with code email_verification_required, a pending acceptance 403
// with code acceptance_pending, a withheld User 403 with code second_factor_required, an invalid
// link 400, incomplete 428 and an unknown reference (a pending sign-up that is gone, a foreign
// cursor) 400.

// refusal is the status and the problem err is answered with, or false when err is not one of the
// refusals a module decides.
func refusal(err error) (int, *siteapi.ProblemDetails, bool) {
	if errors.Is(err, workspaces.ErrNotFound) {
		// An unknown Workspace and one the caller is not a member of read the same.
		return http.StatusNotFound, Body(http.StatusNotFound, "workspace not found"), true
	}
	if used, ok := errors.AsType[*inuse.Error](err); ok {
		return http.StatusConflict, inUseBody(used), true
	}
	// A refused User deletion: its answer always lists the Workspaces the User solely owns.
	if refused, ok := errors.AsType[*users.DeletionRefusedError](err); ok {
		status := kindStatus(refused.Reason.Kind())
		p := fieldBody(status, refused.Reason.Field(), refused.Reason.Message())
		p.Workspaces = mapper.SoleOwnedWorkspaces(refused.Workspaces)
		return status, p, true
	}
	if form, ok := errors.AsType[*validation.FieldsError](err); ok {
		p := Body(http.StatusUnprocessableEntity, form.Detail())
		p.Errors = siteapi.NewOptProblemDetailsErrors(siteapi.ProblemDetailsErrors(form.Fields))
		return http.StatusUnprocessableEntity, p, true
	}
	if invalid, ok := errors.AsType[*validation.Error](err); ok {
		return http.StatusUnprocessableEntity, fieldBody(http.StatusUnprocessableEntity, invalid.Field, invalid.Message), true
	}
	// A module's sentinel refusal: its kind decides the status. The detail is the message of the
	// outermost refusal in the chain, so a refusal made With a detail shows it.
	if refused, ok := errors.AsType[*validation.RefusalError](err); ok {
		status := kindStatus(refused.Kind())
		p := fieldBody(status, refused.Field(), validation.Message(err))
		if code, ok := kindCode(refused.Kind()); ok {
			p.Code = siteapi.NewOptProblemCode(code)
		}
		return status, p, true
	}
	// Content that cannot render is the author's to fix; the error carries the reason.
	if errors.Is(err, emailrender.ErrInvalidContent) {
		return http.StatusUnprocessableEntity, Body(http.StatusUnprocessableEntity, err.Error()), true
	}
	return 0, nil, false
}

// kindStatus is the status every surface answers a refusal of kind with.
func kindStatus(kind validation.Kind) int {
	switch kind {
	case validation.KindNotFound:
		return http.StatusNotFound
	case validation.KindConflict:
		return http.StatusConflict
	case validation.KindForbidden:
		return http.StatusForbidden
	case validation.KindUnauthorized:
		return http.StatusUnauthorized
	case validation.KindDenied, validation.KindUnverified, validation.KindAcceptancePending, validation.KindSecondFactorRequired:
		return http.StatusForbidden
	case validation.KindLinkInvalid, validation.KindUnknownReference:
		return http.StatusBadRequest
	case validation.KindIncomplete:
		return http.StatusPreconditionRequired
	case validation.KindRefused:
		return http.StatusUnprocessableEntity
	}
	return http.StatusUnprocessableEntity
}

// kindCode is the problem code a refusal of kind carries, by which the client tells it from the
// other answers of its status: a Role without the Permission, an address not proven yet, a required
// acceptance pending and a User the Two-factor requirement withholds.
func kindCode(kind validation.Kind) (siteapi.ProblemCode, bool) {
	switch kind {
	case validation.KindForbidden:
		return siteapi.ProblemCodePermissionRequired, true
	case validation.KindUnverified:
		return siteapi.ProblemCodeEmailVerificationRequired, true
	case validation.KindAcceptancePending:
		return siteapi.ProblemCodeAcceptancePending, true
	case validation.KindSecondFactorRequired:
		return siteapi.ProblemCodeSecondFactorRequired, true
	default:
		return "", false
	}
}

// fieldBody is the problem of a refusal that names the request field it is about, or the plain
// problem when field is "".
func fieldBody(status int, field, detail string) *siteapi.ProblemDetails {
	p := Body(status, detail)
	if field != "" {
		p.Errors = siteapi.NewOptProblemDetailsErrors(siteapi.ProblemDetailsErrors{field: {detail}})
	}
	return p
}

// inUseBody is the problem of a removal refused because live work uses the row (ADR 0049):
// `usedBy` names the users, with a count only for an entry that stands for several rows.
func inUseBody(e *inuse.Error) *siteapi.ProblemDetails {
	p := Body(http.StatusConflict, e.Error())
	p.UsedBy = lo.Map(e.Users, func(u inuse.User, _ int) siteapi.UsedBy {
		by := siteapi.UsedBy{Kind: mapper.UsedByKind(u.Kind), ID: siteapi.EntityId(u.ID), Name: u.Name}
		if u.Count > 0 {
			by.Count = siteapi.NewOptInt32(narrow.Int32(u.Count))
		}
		return by
	})
	return p
}
