package site

import (
	"context"

	"github.com/getsphericon/sphericon/ent"
	siteapi "github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/convert"
	"github.com/getsphericon/sphericon/internal/pagination"
)

func (h *Handlers) SiteTagsList(ctx context.Context, params siteapi.SiteTagsListParams) (siteapi.SiteTagsListRes, error) {
	scoped := workspaceScope(ctx)

	all, err := h.tags.List(ctx, scoped)
	if err != nil {
		return nil, err
	}
	page := siteTagsPage(all, params.Page, params.PageSize)
	out := mapper.TagsListPage(page)
	return &out, nil
}

func (h *Handlers) SiteTagsListForContact(ctx context.Context, params siteapi.SiteTagsListForContactParams) (siteapi.SiteTagsListForContactRes, error) {
	scoped := workspaceScope(ctx)
	id := int64(params.ContactId)

	mine, err := h.tags.ForContact(ctx, scoped, id)
	if err != nil {
		return nil, err
	}
	page := siteTagsPage(mine, params.Page, params.PageSize)
	out := mapper.TagsListForContactPage(page)
	return &out, nil
}

func (h *Handlers) SiteTagsApply(ctx context.Context, req *siteapi.ApplyTagInput, params siteapi.SiteTagsApplyParams) (siteapi.SiteTagsApplyRes, error) {
	scoped := workspaceScope(ctx)
	id := int64(params.ContactId)

	t, err := h.tags.Apply(ctx, scoped, id, req.Name)
	if err != nil {
		return nil, err
	}
	resource := mapper.TagToResource(t)
	return &resource, nil
}

func (h *Handlers) SiteTagsRemove(ctx context.Context, params siteapi.SiteTagsRemoveParams) (siteapi.SiteTagsRemoveRes, error) {
	scoped := workspaceScope(ctx)
	id := int64(params.ContactId)

	err := h.tags.Remove(ctx, scoped, id, params.Name)
	if err != nil {
		return nil, err
	}
	return &siteapi.SiteTagsRemoveNoContent{}, nil
}

// SiteTagsDelete deletes a Tag from the catalogue unless a Segment rule or an Automation step
// names it (ADR 0049).
func (h *Handlers) SiteTagsDelete(ctx context.Context, params siteapi.SiteTagsDeleteParams) (siteapi.SiteTagsDeleteRes, error) {
	scoped := workspaceScope(ctx)

	err := h.tags.Delete(ctx, scoped, int64(params.ID))
	if err != nil {
		return nil, err
	}
	return &siteapi.SiteTagsDeleteNoContent{}, nil
}

// siteTagsPage maps and pages a small in-memory tag set.
func siteTagsPage(items []*ent.Tag, page, pageSize siteapi.OptInt32) pagination.Page[siteapi.SiteTagResource] {
	resources := mapper.TagsToResources(items)
	return pagination.Paginate(resources, convert.Ptr(page), convert.Ptr(pageSize))
}
