package site

import (
	"context"
	"errors"
	"github.com/samber/lo"

	"github.com/getsphericon/sphericon/internal/accounts"
	"net/http"
	"strconv"

	"github.com/getsphericon/sphericon/ent"
	siteapi "github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/api/auth"
	"github.com/getsphericon/sphericon/internal/contacts"
	"github.com/getsphericon/sphericon/internal/convert"
	"github.com/getsphericon/sphericon/internal/erasure"
	"github.com/getsphericon/sphericon/internal/pagination"
)

func (h *Handlers) SiteContactsList(ctx context.Context, params siteapi.SiteContactsListParams) (siteapi.SiteContactsListRes, error) {
	scoped, err := h.scopedFor(ctx, params.Slug)
	if ent.IsNotFound(err) {
		v := siteapi.SiteContactsListNotFound(problem(http.StatusNotFound, "workspace not found"))
		return &v, nil
	}
	if err != nil {
		return nil, err
	}

	page, err := h.contacts.List(ctx, scoped, pagination.ParamsOf(params.Page, params.PageSize))
	if err != nil {
		return nil, err
	}

	return &siteapi.SiteContactsListOK{
		Items: lo.Map(page.Items, func(c *ent.Contact, _ int) siteapi.SiteContactResource {
			return mapper.ContactToResource(c)
		}),
		Page:       int32(page.Page),
		PageSize:   int32(page.PageSize),
		TotalItems: int32(page.TotalItems),
		TotalPages: int32(page.TotalPages),
	}, nil
}

func (h *Handlers) SiteContactsCreate(ctx context.Context, req *siteapi.SiteCreateContactInput, params siteapi.SiteContactsCreateParams) (siteapi.SiteContactsCreateRes, error) {
	scoped, err := h.scopedFor(ctx, params.Slug)
	if ent.IsNotFound(err) {
		v := siteapi.SiteContactsCreateNotFound(problem(http.StatusNotFound, "workspace not found"))
		return &v, nil
	}
	if err != nil {
		return nil, err
	}

	c, err := h.contacts.Create(ctx, scoped, createContactAttributes(req))
	var conflict *contacts.ConflictError
	if errors.As(err, &conflict) {
		v := siteapi.SiteContactsCreateConflict(conflictProblem(conflict))
		return &v, nil
	}
	if err != nil {
		return nil, err
	}
	res := mapper.ContactToResource(c)
	return &res, nil
}

func (h *Handlers) SiteContactsGet(ctx context.Context, params siteapi.SiteContactsGetParams) (siteapi.SiteContactsGetRes, error) {
	scoped, err := h.scopedFor(ctx, params.Slug)
	if ent.IsNotFound(err) {
		v := siteapi.SiteContactsGetNotFound(problem(http.StatusNotFound, "workspace not found"))
		return &v, nil
	}
	if err != nil {
		return nil, err
	}

	id, err := strconv.ParseInt(string(params.ID), 10, 64)
	if err != nil {
		v := siteapi.SiteContactsGetBadRequest(problem(http.StatusBadRequest, "invalid id"))
		return &v, nil
	}
	c, err := scoped.Contact().Get(ctx, id)
	if ent.IsNotFound(err) {
		v := siteapi.SiteContactsGetNotFound(problem(http.StatusNotFound, "contact not found"))
		return &v, nil
	}
	if err != nil {
		return nil, err
	}
	res := mapper.ContactToResource(c)
	return &res, nil
}

func (h *Handlers) SiteContactsUpdate(ctx context.Context, req *siteapi.SiteUpdateContactInput, params siteapi.SiteContactsUpdateParams) (siteapi.SiteContactsUpdateRes, error) {
	scoped, err := h.scopedFor(ctx, params.Slug)
	if ent.IsNotFound(err) {
		v := siteapi.SiteContactsUpdateNotFound(problem(http.StatusNotFound, "workspace not found"))
		return &v, nil
	}
	if err != nil {
		return nil, err
	}

	id, err := strconv.ParseInt(string(params.ID), 10, 64)
	if err != nil {
		v := siteapi.SiteContactsUpdateBadRequest(problem(http.StatusBadRequest, "invalid id"))
		return &v, nil
	}
	c, err := h.contacts.Update(ctx, scoped, id, updateContactAttributes(req))
	var conflict *contacts.ConflictError
	if errors.As(err, &conflict) {
		v := siteapi.SiteContactsUpdateConflict(conflictProblem(conflict))
		return &v, nil
	}
	if errors.Is(err, contacts.ErrNotFound) {
		v := siteapi.SiteContactsUpdateNotFound(problem(http.StatusNotFound, "contact not found"))
		return &v, nil
	}
	if err != nil {
		return nil, err
	}
	res := mapper.ContactToResource(c)
	return &res, nil
}

// SiteContactsDelete is Erasure (ADR 0021): irreversible, so owner or admin only.
func (h *Handlers) SiteContactsDelete(ctx context.Context, params siteapi.SiteContactsDeleteParams) (siteapi.SiteContactsDeleteRes, error) {
	scoped, role, err := h.scopedWithRoleFor(ctx, params.Slug)
	if ent.IsNotFound(err) {
		v := siteapi.SiteContactsDeleteNotFound(problem(http.StatusNotFound, "workspace not found"))
		return &v, nil
	}
	if err != nil {
		return nil, err
	}
	if !accounts.CanErase(role) {
		v := siteapi.SiteContactsDeleteForbidden(problem(http.StatusForbidden, "insufficient role"))
		return &v, nil
	}

	id, err := strconv.ParseInt(string(params.ID), 10, 64)
	if err != nil {
		v := siteapi.SiteContactsDeleteBadRequest(problem(http.StatusBadRequest, "invalid id"))
		return &v, nil
	}
	err = h.erasure.Erase(ctx, scoped, erasure.ByContactID(id),
		erasure.StaffMember{Kind: erasure.StaffUser, ID: auth.GetSiteAuth(ctx).UserID})
	if errors.Is(err, erasure.ErrNotFound) {
		v := siteapi.SiteContactsDeleteNotFound(problem(http.StatusNotFound, "contact not found"))
		return &v, nil
	}
	if err != nil {
		return nil, err
	}
	return &siteapi.SiteContactsDeleteNoContent{}, nil
}

// problem builds a ProblemDetails with status, title and detail.
func problem(code int, detail string) siteapi.ProblemDetails {
	return siteapi.ProblemDetails{
		Status: siteapi.NewOptInt32(int32(code)),
		Title:  siteapi.NewOptString(http.StatusText(code)),
		Detail: siteapi.NewOptString(detail),
	}
}

// problemWithErrors builds a ProblemDetails with field-level validation errors.
func problemWithErrors(code int, detail string, errors map[string][]string) siteapi.ProblemDetails {
	p := problem(code, detail)
	p.Errors = siteapi.NewOptProblemDetailsErrors(siteapi.ProblemDetailsErrors(errors))
	return p
}

// conflictProblem renders a contacts.ConflictError, with the field-level error.
func conflictProblem(c *contacts.ConflictError) siteapi.ProblemDetails {
	return problemWithErrors(http.StatusConflict, c.Message(), map[string][]string{c.Field: {c.Message()}})
}

func createContactAttributes(req *siteapi.SiteCreateContactInput) contacts.Attributes {
	attrs := contacts.Attributes{
		SubjectID: convert.StringPtr(req.SubjectId),
		Email:     convert.StringPtr(req.Email),
		Phone:     convert.StringPtr(req.Phone),
		FirstName: convert.StringPtr(req.FirstName),
		LastName:  convert.StringPtr(req.LastName),
		TimeZone:  convert.StringPtr(req.TimeZone),
	}
	if v, ok := req.CustomFields.Get(); ok {
		attrs.CustomFields = convert.RawMap(v)
	}
	return attrs
}

func updateContactAttributes(req *siteapi.SiteUpdateContactInput) contacts.Attributes {
	attrs := contacts.Attributes{
		SubjectID: convert.StringPtr(req.SubjectId),
		Email:     convert.StringPtr(req.Email),
		Phone:     convert.StringPtr(req.Phone),
		FirstName: convert.StringPtr(req.FirstName),
		LastName:  convert.StringPtr(req.LastName),
		TimeZone:  convert.StringPtr(req.TimeZone),
		// JSON Merge Patch: an explicit null clears the field, an absent key keeps it.
		Cleared: contacts.Cleared{
			SubjectID:    req.SubjectId.IsNull(),
			Email:        req.Email.IsNull(),
			Phone:        req.Phone.IsNull(),
			FirstName:    req.FirstName.IsNull(),
			LastName:     req.LastName.IsNull(),
			TimeZone:     req.TimeZone.IsNull(),
			CustomFields: req.CustomFields.IsNull(),
		},
	}
	if v, ok := req.CustomFields.Get(); ok {
		attrs.CustomFields = convert.RawMap(v)
	}
	return attrs
}
