package site

import (
	"context"

	siteapi "github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/domain/content/emailtemplates"
	"github.com/getsphericon/sphericon/internal/kernel/pagination"
)

func (h *Handlers) SiteTemplatesList(ctx context.Context, params siteapi.SiteTemplatesListParams) (*siteapi.SiteTemplatesListOK, error) {
	scoped := workspaceScope(ctx)

	page, err := h.templates.List(ctx, scoped, pagination.ParamsOf(params.Page, params.PageSize))
	if err != nil {
		return nil, err
	}

	return mapper.TemplatesListPage(page), nil
}

func (h *Handlers) SiteTemplatesCreate(ctx context.Context, req *siteapi.CreateTemplateInput, _ siteapi.SiteTemplatesCreateParams) (*siteapi.SiteEmailTemplateResource, error) {
	scoped := workspaceScope(ctx)

	saved, err := h.templateViews.Create(ctx, scoped, inputs.TemplateCreate(req))
	if err != nil {
		return nil, err
	}
	res := savedTemplate(saved)
	return &res, nil
}

func (h *Handlers) SiteTemplatesGet(ctx context.Context, params siteapi.SiteTemplatesGetParams) (*siteapi.SiteEmailTemplateResource, error) {
	scoped := workspaceScope(ctx)

	id := int64(params.ID)
	tpl, err := h.templates.Get(ctx, scoped, id)
	if err != nil {
		return nil, err
	}
	res := mapper.EmailTemplateToResource(tpl)
	return &res, nil
}

func (h *Handlers) SiteTemplatesUpdate(ctx context.Context, req *siteapi.UpdateTemplateInput, params siteapi.SiteTemplatesUpdateParams) (*siteapi.SiteEmailTemplateResource, error) {
	scoped := workspaceScope(ctx)

	id := int64(params.ID)

	saved, err := h.templateViews.Update(ctx, scoped, id, inputs.TemplateUpdate(req))
	if err != nil {
		return nil, err
	}
	res := savedTemplate(saved)
	return &res, nil
}

func (h *Handlers) SiteTemplatesDelete(ctx context.Context, params siteapi.SiteTemplatesDeleteParams) error {
	scoped := workspaceScope(ctx)

	id := int64(params.ID)
	err := h.templates.Delete(ctx, scoped, id)
	if err != nil {
		return err
	}
	return nil
}

// savedTemplate maps a saved Template and attaches the lint warnings of its content (ADR 0040).
func savedTemplate(saved emailtemplates.Saved) siteapi.SiteEmailTemplateResource {
	res := mapper.EmailTemplateToResource(saved.Template)
	res.Warnings = mapper.LintFindingsToResource(saved.Warnings)
	return res
}
