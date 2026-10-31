package site

import (
	"context"

	siteapi "github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/pagination"
)

func (h *Handlers) SiteTemplatesList(ctx context.Context, params siteapi.SiteTemplatesListParams) (siteapi.SiteTemplatesListRes, error) {
	scoped := workspaceScope(ctx)

	page, err := h.templates.List(ctx, scoped, pagination.ParamsOf(params.Page, params.PageSize))
	if err != nil {
		return nil, err
	}

	out := mapper.TemplatesListPage(page)
	return &out, nil
}

func (h *Handlers) SiteTemplatesCreate(ctx context.Context, req *siteapi.CreateTemplateInput, _ siteapi.SiteTemplatesCreateParams) (siteapi.SiteTemplatesCreateRes, error) {
	scoped := workspaceScope(ctx)

	tpl, err := h.templates.Create(ctx, scoped, inputs.TemplateCreate(req))
	if err != nil {
		return nil, err
	}
	res := mapper.EmailTemplateToResource(tpl)
	if res.Warnings, err = h.lintWarnings(ctx, scoped, tpl.Subject, req.Body, tpl.Body); err != nil {
		return nil, err
	}
	return &res, nil
}

func (h *Handlers) SiteTemplatesGet(ctx context.Context, params siteapi.SiteTemplatesGetParams) (siteapi.SiteTemplatesGetRes, error) {
	scoped := workspaceScope(ctx)

	id := int64(params.ID)
	tpl, err := h.templates.Get(ctx, scoped, id)
	if err != nil {
		return nil, err
	}
	res := mapper.EmailTemplateToResource(tpl)
	return &res, nil
}

func (h *Handlers) SiteTemplatesUpdate(ctx context.Context, req *siteapi.UpdateTemplateInput, params siteapi.SiteTemplatesUpdateParams) (siteapi.SiteTemplatesUpdateRes, error) {
	scoped := workspaceScope(ctx)

	id := int64(params.ID)

	tpl, err := h.templates.Update(ctx, scoped, id, inputs.TemplateUpdate(req))
	if err != nil {
		return nil, err
	}
	res := mapper.EmailTemplateToResource(tpl)
	if res.Warnings, err = h.lintWarnings(ctx, scoped, tpl.Subject, req.Body, tpl.Body); err != nil {
		return nil, err
	}
	return &res, nil
}

func (h *Handlers) SiteTemplatesDelete(ctx context.Context, params siteapi.SiteTemplatesDeleteParams) (siteapi.SiteTemplatesDeleteRes, error) {
	scoped := workspaceScope(ctx)

	id := int64(params.ID)
	err := h.templates.Delete(ctx, scoped, id)
	if err != nil {
		return nil, err
	}
	return &siteapi.SiteTemplatesDeleteNoContent{}, nil
}
