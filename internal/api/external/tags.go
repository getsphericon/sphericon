package external

import (
	"context"

	externalapi "github.com/getsphericon/sphericon/gen/external"
	"github.com/getsphericon/sphericon/internal/api/auth"
	"github.com/getsphericon/sphericon/internal/convert"
	"github.com/getsphericon/sphericon/internal/pagination"
)

func (h *Handlers) TagsList(ctx context.Context, params externalapi.TagsListParams) (externalapi.TagsListRes, error) {
	scoped := auth.TokenScoped(ctx)
	all, err := h.tags.List(ctx, scoped)
	if err != nil {
		return nil, err
	}
	return mapper.TagsListPage(pagination.Paginate(all, convert.Ptr(params.Page), convert.Ptr(params.PageSize))), nil
}

func (h *Handlers) TagsListForContact(ctx context.Context, params externalapi.TagsListForContactParams) (externalapi.TagsListForContactRes, error) {
	id := int64(params.ContactId)

	scoped := auth.TokenScoped(ctx)
	mine, err := h.tags.ForContact(ctx, scoped, id)
	if err != nil {
		return nil, err
	}
	return mapper.TagsListForContactPage(pagination.Paginate(mine, convert.Ptr(params.Page), convert.Ptr(params.PageSize))), nil
}

func (h *Handlers) TagsApply(ctx context.Context, req *externalapi.ApplyTagInput, params externalapi.TagsApplyParams) (externalapi.TagsApplyRes, error) {
	id := int64(params.ContactId)

	scoped := auth.TokenScoped(ctx)
	t, err := h.tags.Apply(ctx, scoped, id, req.Name)
	if err != nil {
		return nil, err
	}
	resource := mapper.TagToResource(t)
	return &resource, nil
}

func (h *Handlers) TagsRemove(ctx context.Context, params externalapi.TagsRemoveParams) (externalapi.TagsRemoveRes, error) {
	id := int64(params.ContactId)

	scoped := auth.TokenScoped(ctx)
	err := h.tags.Remove(ctx, scoped, id, params.Name)
	if err != nil {
		return nil, err
	}
	return &externalapi.TagsRemoveNoContent{}, nil
}

// TagsDelete deletes a Tag from the catalogue unless a Segment rule or an Automation step names
// it (ADR 0049).
func (h *Handlers) TagsDelete(ctx context.Context, params externalapi.TagsDeleteParams) (externalapi.TagsDeleteRes, error) {
	err := h.tags.Delete(ctx, auth.TokenScoped(ctx), int64(params.ID))
	if err != nil {
		return nil, err
	}
	return &externalapi.TagsDeleteNoContent{}, nil
}
