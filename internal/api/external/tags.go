package external

import (
	"context"
	"errors"
	"net/http"

	"github.com/samber/lo"

	"github.com/getsphericon/sphericon/ent"
	externalapi "github.com/getsphericon/sphericon/gen/external"
	"github.com/getsphericon/sphericon/internal/api/auth"
	"github.com/getsphericon/sphericon/internal/convert"
	"github.com/getsphericon/sphericon/internal/inuse"
	"github.com/getsphericon/sphericon/internal/narrow"
	"github.com/getsphericon/sphericon/internal/pagination"
	"github.com/getsphericon/sphericon/internal/tags"
	"github.com/getsphericon/sphericon/internal/validation"
)

func (h *Handlers) TagsList(ctx context.Context, params externalapi.TagsListParams) (externalapi.TagsListRes, error) {
	if !auth.HasScope(auth.GetTokenAuth(ctx), externalapi.ApiTokenScopeContactsRead) {
		res := externalapi.TagsListUnauthorized(problem(http.StatusUnauthorized, "insufficient scope"))
		return &res, nil
	}

	scoped := auth.TokenScoped(ctx)
	all, err := h.tags.List(ctx, scoped)
	if err != nil {
		return nil, err
	}
	page := tagsPage(all, params.Page, params.PageSize)
	return &externalapi.TagsListOK{
		Items:      page.Items,
		Page:       narrow.Int32(page.Page),
		PageSize:   narrow.Int32(page.PageSize),
		TotalItems: narrow.Int32(page.TotalItems),
		TotalPages: narrow.Int32(page.TotalPages),
	}, nil
}

func (h *Handlers) TagsListForContact(ctx context.Context, params externalapi.TagsListForContactParams) (externalapi.TagsListForContactRes, error) {
	if !auth.HasScope(auth.GetTokenAuth(ctx), externalapi.ApiTokenScopeContactsRead) {
		res := externalapi.TagsListForContactUnauthorized(problem(http.StatusUnauthorized, "insufficient scope"))
		return &res, nil
	}
	id := int64(params.ContactId)

	scoped := auth.TokenScoped(ctx)
	mine, err := h.tags.ForContact(ctx, scoped, id)
	if errors.Is(err, tags.ErrContactNotFound) {
		res := externalapi.TagsListForContactNotFound(problem(http.StatusNotFound, validation.Message(err)))
		return &res, nil
	}
	if err != nil {
		return nil, err
	}
	page := tagsPage(mine, params.Page, params.PageSize)
	return &externalapi.TagsListForContactOK{
		Items:      page.Items,
		Page:       narrow.Int32(page.Page),
		PageSize:   narrow.Int32(page.PageSize),
		TotalItems: narrow.Int32(page.TotalItems),
		TotalPages: narrow.Int32(page.TotalPages),
	}, nil
}

func (h *Handlers) TagsApply(ctx context.Context, req *externalapi.ApplyTagInput, params externalapi.TagsApplyParams) (externalapi.TagsApplyRes, error) {
	if !auth.HasScope(auth.GetTokenAuth(ctx), externalapi.ApiTokenScopeContactsWrite) {
		res := externalapi.TagsApplyUnauthorized(problem(http.StatusUnauthorized, "insufficient scope"))
		return &res, nil
	}
	id := int64(params.ContactId)

	scoped := auth.TokenScoped(ctx)
	t, err := h.tags.Apply(ctx, scoped, id, req.Name)
	if errors.Is(err, tags.ErrContactNotFound) {
		res := externalapi.TagsApplyNotFound(problem(http.StatusNotFound, validation.Message(err)))
		return &res, nil
	}
	if errors.Is(err, tags.ErrInvalidName) {
		res := externalapi.TagsApplyUnprocessableEntity(problem(http.StatusUnprocessableEntity, validation.Message(err)))
		return &res, nil
	}
	if err != nil {
		return nil, err
	}
	resource := mapper.TagToResource(t)
	return &resource, nil
}

func (h *Handlers) TagsRemove(ctx context.Context, params externalapi.TagsRemoveParams) (externalapi.TagsRemoveRes, error) {
	if !auth.HasScope(auth.GetTokenAuth(ctx), externalapi.ApiTokenScopeContactsWrite) {
		res := externalapi.TagsRemoveUnauthorized(problem(http.StatusUnauthorized, "insufficient scope"))
		return &res, nil
	}
	id := int64(params.ContactId)

	scoped := auth.TokenScoped(ctx)
	err := h.tags.Remove(ctx, scoped, id, params.Name)
	if errors.Is(err, tags.ErrContactNotFound) {
		res := externalapi.TagsRemoveNotFound(problem(http.StatusNotFound, validation.Message(err)))
		return &res, nil
	}
	if err != nil {
		return nil, err
	}
	return &externalapi.TagsRemoveNoContent{}, nil
}

// TagsDelete deletes a Tag from the catalogue unless a Segment rule or an Automation step names
// it (ADR 0049).
func (h *Handlers) TagsDelete(ctx context.Context, params externalapi.TagsDeleteParams) (externalapi.TagsDeleteRes, error) {
	if !auth.HasScope(auth.GetTokenAuth(ctx), externalapi.ApiTokenScopeContactsWrite) {
		res := externalapi.TagsDeleteUnauthorized(problem(http.StatusUnauthorized, "insufficient scope"))
		return &res, nil
	}
	err := h.tags.Delete(ctx, auth.TokenScoped(ctx), int64(params.ID))
	if refused, ok := inuse.As(err); ok {
		res := inUseProblem(refused)
		return &res, nil
	}
	if errors.Is(err, tags.ErrNotFound) {
		res := externalapi.TagsDeleteNotFound(problem(http.StatusNotFound, validation.Message(err)))
		return &res, nil
	}
	if err != nil {
		return nil, err
	}
	return &externalapi.TagsDeleteNoContent{}, nil
}

// tagsPage maps and pages a small in-memory tag set.
func tagsPage(items []*ent.Tag, page, pageSize externalapi.OptInt32) pagination.Page[externalapi.TagResource] {
	resources := lo.Map(items, func(t *ent.Tag, _ int) externalapi.TagResource { return mapper.TagToResource(t) })
	return pagination.Paginate(resources, convert.Ptr(page), convert.Ptr(pageSize))
}
