package external

import (
	"context"

	externalapi "github.com/getsphericon/sphericon/gen/external"
	"github.com/getsphericon/sphericon/internal/api/auth"
	"github.com/getsphericon/sphericon/internal/pagination"
)

func (h *Handlers) TemplatesList(ctx context.Context, params externalapi.TemplatesListParams) (externalapi.TemplatesListRes, error) {
	page, err := h.templates.List(ctx, auth.TokenScoped(ctx), pagination.ParamsOf(params.Page, params.PageSize))
	if err != nil {
		return nil, err
	}
	return mapper.TemplatesListPage(page), nil
}

func (h *Handlers) TemplatesCreate(ctx context.Context, req *externalapi.CreateTemplateInput) (externalapi.TemplatesCreateRes, error) {
	scoped := auth.TokenScoped(ctx)
	tpl, err := h.templates.Create(ctx, scoped, inputs.TemplateCreate(req))
	if err != nil {
		return nil, err
	}
	res := mapper.EmailTemplateToResource(tpl)
	return &res, nil
}

func (h *Handlers) TemplatesGet(ctx context.Context, params externalapi.TemplatesGetParams) (externalapi.TemplatesGetRes, error) {
	id := int64(params.ID)

	scoped := auth.TokenScoped(ctx)
	tpl, err := h.templates.Get(ctx, scoped, id)
	if err != nil {
		return nil, err
	}
	res := mapper.EmailTemplateToResource(tpl)
	return &res, nil
}

func (h *Handlers) TemplatesUpdate(ctx context.Context, req *externalapi.UpdateTemplateInput, params externalapi.TemplatesUpdateParams) (externalapi.TemplatesUpdateRes, error) {
	id := int64(params.ID)

	scoped := auth.TokenScoped(ctx)
	tpl, err := h.templates.Update(ctx, scoped, id, inputs.TemplateUpdate(req))
	if err != nil {
		return nil, err
	}
	res := mapper.EmailTemplateToResource(tpl)
	return &res, nil
}

func (h *Handlers) TemplatesDelete(ctx context.Context, params externalapi.TemplatesDeleteParams) (externalapi.TemplatesDeleteRes, error) {
	id := int64(params.ID)

	scoped := auth.TokenScoped(ctx)
	err := h.templates.Delete(ctx, scoped, id)
	if err != nil {
		return nil, err
	}
	return &externalapi.TemplatesDeleteNoContent{}, nil
}
