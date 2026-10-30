package site

import (
	"context"
	"errors"

	"github.com/samber/lo"

	"net/http"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/contact"
	siteapi "github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/api/auth"
	"github.com/getsphericon/sphericon/internal/contacts"
	"github.com/getsphericon/sphericon/internal/convert"
	"github.com/getsphericon/sphericon/internal/erasure"
	"github.com/getsphericon/sphericon/internal/narrow"
	"github.com/getsphericon/sphericon/internal/pagination"
)

func (h *Handlers) SiteContactsList(ctx context.Context, params siteapi.SiteContactsListParams) (siteapi.SiteContactsListRes, error) {
	scoped := workspaceScope(ctx)

	page, err := h.contacts.List(ctx, scoped, pagination.ParamsOf(params.Page, params.PageSize), contacts.Filter{EmailContains: params.Email.Or("")})
	if err != nil {
		return nil, err
	}

	return &siteapi.SiteContactsListOK{
		Items: lo.Map(page.Items, func(c *ent.Contact, _ int) siteapi.SiteContactResource {
			return mapper.ContactToResource(c)
		}),
		Page:       narrow.Int32(page.Page),
		PageSize:   narrow.Int32(page.PageSize),
		TotalItems: narrow.Int32(page.TotalItems),
		TotalPages: narrow.Int32(page.TotalPages),
	}, nil
}

func (h *Handlers) SiteContactsCreate(ctx context.Context, req *siteapi.SiteCreateContactInput, _ siteapi.SiteContactsCreateParams) (siteapi.SiteContactsCreateRes, error) {
	scoped := workspaceScope(ctx)

	c, err := h.contacts.Create(ctx, scoped, contact.SourceManual, createContactAttributes(req))
	if conflict, ok := errors.AsType[*contacts.ConflictError](err); ok {
		v := siteapi.SiteContactsCreateConflict(conflictProblem(conflict))
		return &v, nil
	}
	if err != nil {
		return nil, err
	}
	res, err := h.contactResource(ctx, scoped, c.ID, req.Tags)
	if err != nil {
		return nil, err
	}
	return &res, nil
}

func (h *Handlers) SiteContactsGet(ctx context.Context, params siteapi.SiteContactsGetParams) (siteapi.SiteContactsGetRes, error) {
	scoped := workspaceScope(ctx)

	res, err := h.contactResource(ctx, scoped, int64(params.ID), nil)
	if errors.Is(err, contacts.ErrNotFound) {
		v := siteapi.SiteContactsGetNotFound(problem(http.StatusNotFound, "contact not found"))
		return &v, nil
	}
	if err != nil {
		return nil, err
	}
	return &res, nil
}

func (h *Handlers) SiteContactsUpdate(ctx context.Context, req *siteapi.SiteUpdateContactInput, params siteapi.SiteContactsUpdateParams) (siteapi.SiteContactsUpdateRes, error) {
	scoped := workspaceScope(ctx)

	id := int64(params.ID)
	c, err := h.contacts.Update(ctx, scoped, id, updateContactAttributes(req))
	if conflict, ok := errors.AsType[*contacts.ConflictError](err); ok {
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
	res, err := h.contactResource(ctx, scoped, c.ID, req.Tags)
	if err != nil {
		return nil, err
	}
	return &res, nil
}

// SiteContactsDelete is Erasure (ADR 0021): irreversible, so owner or admin only.
func (h *Handlers) SiteContactsDelete(ctx context.Context, params siteapi.SiteContactsDeleteParams) (siteapi.SiteContactsDeleteRes, error) {
	scoped := workspaceScope(ctx)

	id := int64(params.ID)
	err := h.erasure.Erase(ctx, scoped, erasure.ByContactID(id),
		erasure.Requester{Kind: erasure.RequesterUser, ID: auth.GetSiteAuth(ctx).UserID})
	if errors.Is(err, erasure.ErrNotFound) {
		v := siteapi.SiteContactsDeleteNotFound(problem(http.StatusNotFound, "contact not found"))
		return &v, nil
	}
	if err != nil {
		return nil, err
	}
	return &siteapi.SiteContactsDeleteNoContent{}, nil
}

// conflictProblem renders a contacts.ConflictError, with the field-level error.
func conflictProblem(c *contacts.ConflictError) siteapi.ProblemDetails {
	return problemWithErrors(http.StatusConflict, c.Message(), map[string][]string{c.Field: {c.Message()}})
}

// contactResource reads the contact with its tags. A non-nil setTags first makes it the
// contact's complete tag set (an absent key in the request is nil and keeps the tags).
func (h *Handlers) contactResource(ctx context.Context, s *ent.Scoped, id int64, setTags []string) (siteapi.SiteContactResource, error) {
	if setTags != nil {
		if err := h.tags.Set(ctx, s, id, setTags); err != nil {
			return siteapi.SiteContactResource{}, err
		}
	}
	c, err := h.contacts.Get(ctx, s, id)
	if err != nil {
		return siteapi.SiteContactResource{}, err
	}
	return mapper.ContactToResource(c), nil
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
