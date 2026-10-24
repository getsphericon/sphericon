package site

import (
	"context"
	"errors"
	"net/http"

	"github.com/samber/lo"

	"github.com/getsphericon/sphericon/ent"
	siteapi "github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/convert"
	"github.com/getsphericon/sphericon/internal/inuse"
	"github.com/getsphericon/sphericon/internal/narrow"
	"github.com/getsphericon/sphericon/internal/pagination"
	"github.com/getsphericon/sphericon/internal/tags"
	"github.com/getsphericon/sphericon/internal/validation"
)

func (h *Handlers) SiteTagsList(ctx context.Context, params siteapi.SiteTagsListParams) (siteapi.SiteTagsListRes, error) {
	scoped, err := h.scopedFor(ctx, params.WorkspaceId)
	if ent.IsNotFound(err) {
		v := siteapi.SiteTagsListNotFound(problem(http.StatusNotFound, "workspace not found"))
		return &v, nil
	}
	if err != nil {
		return nil, err
	}

	all, err := h.tags.List(ctx, scoped)
	if err != nil {
		return nil, err
	}
	page := siteTagsPage(all, params.Page, params.PageSize)
	return &siteapi.SiteTagsListOK{
		Items:      page.Items,
		Page:       narrow.Int32(page.Page),
		PageSize:   narrow.Int32(page.PageSize),
		TotalItems: narrow.Int32(page.TotalItems),
		TotalPages: narrow.Int32(page.TotalPages),
	}, nil
}

func (h *Handlers) SiteTagsListForContact(ctx context.Context, params siteapi.SiteTagsListForContactParams) (siteapi.SiteTagsListForContactRes, error) {
	scoped, err := h.scopedFor(ctx, params.WorkspaceId)
	if ent.IsNotFound(err) {
		v := siteapi.SiteTagsListForContactNotFound(problem(http.StatusNotFound, "workspace not found"))
		return &v, nil
	}
	if err != nil {
		return nil, err
	}
	id := int64(params.ContactId)

	mine, err := h.tags.ForContact(ctx, scoped, id)
	if errors.Is(err, tags.ErrContactNotFound) {
		v := siteapi.SiteTagsListForContactNotFound(problem(http.StatusNotFound, validation.Message(err)))
		return &v, nil
	}
	if err != nil {
		return nil, err
	}
	page := siteTagsPage(mine, params.Page, params.PageSize)
	return &siteapi.SiteTagsListForContactOK{
		Items:      page.Items,
		Page:       narrow.Int32(page.Page),
		PageSize:   narrow.Int32(page.PageSize),
		TotalItems: narrow.Int32(page.TotalItems),
		TotalPages: narrow.Int32(page.TotalPages),
	}, nil
}

func (h *Handlers) SiteTagsApply(ctx context.Context, req *siteapi.SiteApplyTagInput, params siteapi.SiteTagsApplyParams) (siteapi.SiteTagsApplyRes, error) {
	scoped, err := h.scopedFor(ctx, params.WorkspaceId)
	if ent.IsNotFound(err) {
		v := siteapi.SiteTagsApplyNotFound(problem(http.StatusNotFound, "workspace not found"))
		return &v, nil
	}
	if err != nil {
		return nil, err
	}
	id := int64(params.ContactId)

	t, err := h.tags.Apply(ctx, scoped, id, req.Name)
	if errors.Is(err, tags.ErrContactNotFound) {
		v := siteapi.SiteTagsApplyNotFound(problem(http.StatusNotFound, validation.Message(err)))
		return &v, nil
	}
	if errors.Is(err, tags.ErrInvalidName) {
		v := siteapi.SiteTagsApplyUnprocessableEntity(problem(http.StatusUnprocessableEntity, validation.Message(err)))
		return &v, nil
	}
	if err != nil {
		return nil, err
	}
	resource := mapper.TagToResource(t)
	return &resource, nil
}

func (h *Handlers) SiteTagsRemove(ctx context.Context, params siteapi.SiteTagsRemoveParams) (siteapi.SiteTagsRemoveRes, error) {
	scoped, err := h.scopedFor(ctx, params.WorkspaceId)
	if ent.IsNotFound(err) {
		v := siteapi.SiteTagsRemoveNotFound(problem(http.StatusNotFound, "workspace not found"))
		return &v, nil
	}
	if err != nil {
		return nil, err
	}
	id := int64(params.ContactId)

	err = h.tags.Remove(ctx, scoped, id, params.Name)
	if errors.Is(err, tags.ErrContactNotFound) {
		v := siteapi.SiteTagsRemoveNotFound(problem(http.StatusNotFound, validation.Message(err)))
		return &v, nil
	}
	if err != nil {
		return nil, err
	}
	return &siteapi.SiteTagsRemoveNoContent{}, nil
}

// SiteTagsDelete deletes a Tag from the catalogue unless a Segment rule or an Automation step
// names it (ADR 0049).
func (h *Handlers) SiteTagsDelete(ctx context.Context, params siteapi.SiteTagsDeleteParams) (siteapi.SiteTagsDeleteRes, error) {
	scoped, err := h.scopedFor(ctx, params.WorkspaceId)
	if ent.IsNotFound(err) {
		v := siteapi.SiteTagsDeleteNotFound(problem(http.StatusNotFound, "workspace not found"))
		return &v, nil
	}
	if err != nil {
		return nil, err
	}

	err = h.tags.Delete(ctx, scoped, int64(params.ID))
	if refused, ok := inuse.As(err); ok {
		v := inUseProblem(refused)
		return &v, nil
	}
	if errors.Is(err, tags.ErrNotFound) {
		v := siteapi.SiteTagsDeleteNotFound(problem(http.StatusNotFound, validation.Message(err)))
		return &v, nil
	}
	if err != nil {
		return nil, err
	}
	return &siteapi.SiteTagsDeleteNoContent{}, nil
}

// siteTagsPage maps and pages a small in-memory tag set.
func siteTagsPage(items []*ent.Tag, page, pageSize siteapi.OptInt32) pagination.Page[siteapi.SiteTagResource] {
	resources := lo.Map(items, func(t *ent.Tag, _ int) siteapi.SiteTagResource { return mapper.TagToResource(t) })
	return pagination.Paginate(resources, convert.Ptr(page), convert.Ptr(pageSize))
}
