package site

import (
	"context"

	siteapi "github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/kernel/convert"
	"github.com/getsphericon/sphericon/internal/kernel/pagination"
)

func (h *Handlers) SiteTagsList(ctx context.Context, params siteapi.SiteTagsListParams) (*siteapi.SiteTagsListOK, error) {
	scoped := workspaceScope(ctx)

	all, err := h.tags.List(ctx, scoped)
	if err != nil {
		return nil, err
	}
	return mapper.TagsListPage(pagination.Paginate(all, convert.Ptr(params.Page), convert.Ptr(params.PageSize))), nil
}

func (h *Handlers) SiteTagsListForContact(ctx context.Context, params siteapi.SiteTagsListForContactParams) (*siteapi.SiteTagsListForContactOK, error) {
	scoped := workspaceScope(ctx)
	id := int64(params.ContactId)

	mine, err := h.tags.ForContact(ctx, scoped, id)
	if err != nil {
		return nil, err
	}
	return mapper.TagsListForContactPage(pagination.Paginate(mine, convert.Ptr(params.Page), convert.Ptr(params.PageSize))), nil
}

func (h *Handlers) SiteTagsApply(ctx context.Context, req *siteapi.ApplyTagInput, params siteapi.SiteTagsApplyParams) (*siteapi.SiteTagResource, error) {
	scoped := workspaceScope(ctx)
	id := int64(params.ContactId)

	t, err := h.tags.Apply(ctx, scoped, id, req.Name)
	if err != nil {
		return nil, err
	}
	resource := mapper.TagToResource(t)
	return &resource, nil
}

func (h *Handlers) SiteTagsRemove(ctx context.Context, params siteapi.SiteTagsRemoveParams) error {
	scoped := workspaceScope(ctx)
	id := int64(params.ContactId)

	err := h.tags.Remove(ctx, scoped, id, params.Name)
	if err != nil {
		return err
	}
	return nil
}

// SiteTagsDelete deletes a Tag from the catalogue through the Tag deletion use case, which refuses
// while a Segment, an Automation, a Rule or an armed Broadcast names it (ADR 0049).
func (h *Handlers) SiteTagsDelete(ctx context.Context, params siteapi.SiteTagsDeleteParams) error {
	scoped := workspaceScope(ctx)

	err := h.tagDeletion.Delete(ctx, scoped, int64(params.ID))
	if err != nil {
		return err
	}
	return nil
}
