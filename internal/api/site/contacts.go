package site

import (
	"context"

	"github.com/getsphericon/sphericon/ent/contact"
	siteapi "github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/api/auth"
	"github.com/getsphericon/sphericon/internal/contacts"
	"github.com/getsphericon/sphericon/internal/erasure"
	"github.com/getsphericon/sphericon/internal/pagination"
)

func (h *Handlers) SiteContactsList(ctx context.Context, params siteapi.SiteContactsListParams) (siteapi.SiteContactsListRes, error) {
	scoped := workspaceScope(ctx)

	page, err := h.contacts.List(ctx, scoped, pagination.ParamsOf(params.Page, params.PageSize), contacts.Filter{
		EmailContains: params.Email.Or(""),
		Tag:           params.Tag.Or(""),
		Country:       string(params.Country.Or("")),
	})
	if err != nil {
		return nil, err
	}

	return mapper.ContactsListPage(page), nil
}

func (h *Handlers) SiteContactsCreate(ctx context.Context, req *siteapi.SiteCreateContactInput, _ siteapi.SiteContactsCreateParams) (siteapi.SiteContactsCreateRes, error) {
	scoped := workspaceScope(ctx)

	c, err := h.contacts.Create(ctx, scoped, contact.SourceManual, inputs.ContactCreate(req))
	if err != nil {
		return nil, err
	}
	res := mapper.ContactToResource(c)
	return &res, nil
}

func (h *Handlers) SiteContactsGet(ctx context.Context, params siteapi.SiteContactsGetParams) (siteapi.SiteContactsGetRes, error) {
	scoped := workspaceScope(ctx)

	c, err := h.contacts.Get(ctx, scoped, int64(params.ID))
	if err != nil {
		return nil, err
	}
	res := mapper.ContactToResource(c)
	return &res, nil
}

func (h *Handlers) SiteContactsUpdate(ctx context.Context, req *siteapi.SiteUpdateContactInput, params siteapi.SiteContactsUpdateParams) (siteapi.SiteContactsUpdateRes, error) {
	scoped := workspaceScope(ctx)

	id := int64(params.ID)
	c, err := h.contacts.Update(ctx, scoped, id, inputs.ContactUpdate(req))
	if err != nil {
		return nil, err
	}
	res := mapper.ContactToResource(c)
	return &res, nil
}

// SiteContactsDelete is Erasure (ADR 0021): irreversible, so owner or admin only.
func (h *Handlers) SiteContactsDelete(ctx context.Context, params siteapi.SiteContactsDeleteParams) (siteapi.SiteContactsDeleteRes, error) {
	scoped := workspaceScope(ctx)

	id := int64(params.ID)
	err := h.erasure.Erase(ctx, scoped, erasure.ByContactID(id),
		erasure.Requester{Kind: erasure.RequesterUser, ID: auth.GetSiteAuth(ctx).UserID})
	if err != nil {
		return nil, err
	}
	return &siteapi.SiteContactsDeleteNoContent{}, nil
}
