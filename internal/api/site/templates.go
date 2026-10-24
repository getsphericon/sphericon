package site

import (
	"context"
	"errors"
	"net/http"

	"github.com/getsphericon/sphericon/ent"
	siteapi "github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/convert"
	"github.com/getsphericon/sphericon/internal/emailrender"
	"github.com/getsphericon/sphericon/internal/narrow"
	"github.com/getsphericon/sphericon/internal/pagination"
	"github.com/getsphericon/sphericon/internal/templates"
	"github.com/getsphericon/sphericon/internal/validation"
	"github.com/samber/lo"
)

func (h *Handlers) SiteTemplatesList(ctx context.Context, params siteapi.SiteTemplatesListParams) (siteapi.SiteTemplatesListRes, error) {
	scoped, err := h.scopedFor(ctx, params.WorkspaceId)
	if ent.IsNotFound(err) {
		v := siteapi.SiteTemplatesListNotFound(problem(http.StatusNotFound, "workspace not found"))
		return &v, nil
	}
	if err != nil {
		return nil, err
	}

	page, err := h.templates.List(ctx, scoped, pagination.ParamsOf(params.Page, params.PageSize))
	if err != nil {
		return nil, err
	}

	return &siteapi.SiteTemplatesListOK{
		Items: lo.Map(page.Items, func(t *ent.EmailTemplate, _ int) siteapi.SiteEmailTemplateResource {
			return mapper.EmailTemplateToResource(t)
		}),
		Page:       narrow.Int32(page.Page),
		PageSize:   narrow.Int32(page.PageSize),
		TotalItems: narrow.Int32(page.TotalItems),
		TotalPages: narrow.Int32(page.TotalPages),
	}, nil
}

func (h *Handlers) SiteTemplatesCreate(ctx context.Context, req *siteapi.SiteCreateEmailTemplateInput, params siteapi.SiteTemplatesCreateParams) (siteapi.SiteTemplatesCreateRes, error) {
	scoped, err := h.scopedFor(ctx, params.WorkspaceId)
	if ent.IsNotFound(err) {
		v := siteapi.SiteTemplatesCreateNotFound(problem(http.StatusNotFound, "workspace not found"))
		return &v, nil
	}
	if err != nil {
		return nil, err
	}

	tpl, err := h.templates.Create(ctx, scoped, templates.CreateInput{
		Name:    req.Name,
		Subject: convert.StringPtr(req.Subject),
		Body:    convert.StringPtr(req.Body),
		Doc:     convert.DocJSON(req.Doc.Value, req.Doc.Set),
	})
	if errors.Is(err, templates.ErrBlankName) {
		v := siteapi.SiteTemplatesCreateUnprocessableEntity(problem(http.StatusUnprocessableEntity, validation.Message(err)))
		return &v, nil
	}
	if errors.Is(err, templates.ErrBodyAndDoc) || errors.Is(err, emailrender.ErrInvalidContent) {
		v := siteapi.SiteTemplatesCreateUnprocessableEntity(problem(http.StatusUnprocessableEntity, validation.Message(err)))
		return &v, nil
	}
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
	scoped, err := h.scopedFor(ctx, params.WorkspaceId)
	if ent.IsNotFound(err) {
		v := siteapi.SiteTemplatesGetNotFound(problem(http.StatusNotFound, "workspace not found"))
		return &v, nil
	}
	if err != nil {
		return nil, err
	}

	id := int64(params.ID)
	tpl, err := h.templates.Get(ctx, scoped, id)
	if errors.Is(err, templates.ErrNotFound) {
		v := siteapi.SiteTemplatesGetNotFound(problem(http.StatusNotFound, validation.Message(err)))
		return &v, nil
	}
	if err != nil {
		return nil, err
	}
	res := mapper.EmailTemplateToResource(tpl)
	return &res, nil
}

func (h *Handlers) SiteTemplatesUpdate(ctx context.Context, req *siteapi.SiteUpdateEmailTemplateInput, params siteapi.SiteTemplatesUpdateParams) (siteapi.SiteTemplatesUpdateRes, error) {
	scoped, err := h.scopedFor(ctx, params.WorkspaceId)
	if ent.IsNotFound(err) {
		v := siteapi.SiteTemplatesUpdateNotFound(problem(http.StatusNotFound, "workspace not found"))
		return &v, nil
	}
	if err != nil {
		return nil, err
	}

	id := int64(params.ID)

	tpl, err := h.templates.Update(ctx, scoped, id, templates.UpdateInput{
		Name:    convert.StringPtr(req.Name),
		Subject: convert.StringPtr(req.Subject),
		Body:    convert.StringPtr(req.Body),
		Doc:     convert.DocJSON(req.Doc.Value, req.Doc.Set),
	})
	if errors.Is(err, templates.ErrNotFound) {
		v := siteapi.SiteTemplatesUpdateNotFound(problem(http.StatusNotFound, validation.Message(err)))
		return &v, nil
	}
	if errors.Is(err, templates.ErrBlankName) {
		v := siteapi.SiteTemplatesUpdateUnprocessableEntity(problem(http.StatusUnprocessableEntity, validation.Message(err)))
		return &v, nil
	}
	if errors.Is(err, templates.ErrBodyAndDoc) || errors.Is(err, emailrender.ErrInvalidContent) {
		v := siteapi.SiteTemplatesUpdateUnprocessableEntity(problem(http.StatusUnprocessableEntity, validation.Message(err)))
		return &v, nil
	}
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
	scoped, err := h.scopedFor(ctx, params.WorkspaceId)
	if ent.IsNotFound(err) {
		v := siteapi.SiteTemplatesDeleteNotFound(problem(http.StatusNotFound, "workspace not found"))
		return &v, nil
	}
	if err != nil {
		return nil, err
	}

	id := int64(params.ID)
	err = h.templates.Delete(ctx, scoped, id)
	if errors.Is(err, templates.ErrNotFound) {
		v := siteapi.SiteTemplatesDeleteNotFound(problem(http.StatusNotFound, validation.Message(err)))
		return &v, nil
	}
	if err != nil {
		return nil, err
	}
	return &siteapi.SiteTemplatesDeleteNoContent{}, nil
}
