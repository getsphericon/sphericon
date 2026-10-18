package external

import (
	"context"
	"errors"
	"net/http"

	"github.com/getsphericon/sphericon/ent"
	externalapi "github.com/getsphericon/sphericon/gen/external"
	"github.com/getsphericon/sphericon/internal/api/auth"
	"github.com/getsphericon/sphericon/internal/convert"
	"github.com/getsphericon/sphericon/internal/pagination"
	"github.com/getsphericon/sphericon/internal/templates"
	"github.com/samber/lo"
)

func (h *Handlers) TemplatesList(ctx context.Context, params externalapi.TemplatesListParams) (externalapi.TemplatesListRes, error) {
	if !auth.HasScope(auth.GetTokenAuth(ctx), "templates:read") {
		res := externalapi.TemplatesListUnauthorized(problem(http.StatusUnauthorized, "insufficient scope"))
		return &res, nil
	}

	page, err := h.templates.List(ctx, auth.TokenScoped(ctx), pagination.ParamsOf(params.Page, params.PageSize))
	if err != nil {
		return nil, err
	}
	return &externalapi.TemplatesListOK{
		Items: lo.Map(page.Items, func(t *ent.EmailTemplate, _ int) externalapi.TemplateResource {
			return mapper.EmailTemplateToResource(t)
		}),
		Page:       int32(page.Page),
		PageSize:   int32(page.PageSize),
		TotalItems: int32(page.TotalItems),
		TotalPages: int32(page.TotalPages),
	}, nil
}

func (h *Handlers) TemplatesCreate(ctx context.Context, req *externalapi.CreateTemplateInput) (externalapi.TemplatesCreateRes, error) {
	if !auth.HasScope(auth.GetTokenAuth(ctx), "templates:write") {
		res := externalapi.TemplatesCreateUnauthorized(problem(http.StatusUnauthorized, "insufficient scope"))
		return &res, nil
	}

	scoped := auth.TokenScoped(ctx)
	tpl, err := h.templates.Create(ctx, scoped, templates.CreateInput{
		Name:    req.Name,
		Subject: convert.StringPtr(req.Subject),
		Body:    convert.StringPtr(req.Body),
	})
	if errors.Is(err, templates.ErrBlankName) {
		res := externalapi.TemplatesCreateUnprocessableEntity(problem(http.StatusUnprocessableEntity, "name must not be empty"))
		return &res, nil
	}
	if err != nil {
		return nil, err
	}
	res := mapper.EmailTemplateToResource(tpl)
	return &res, nil
}

func (h *Handlers) TemplatesGet(ctx context.Context, params externalapi.TemplatesGetParams) (externalapi.TemplatesGetRes, error) {
	if !auth.HasScope(auth.GetTokenAuth(ctx), "templates:read") {
		res := externalapi.TemplatesGetUnauthorized(problem(http.StatusUnauthorized, "insufficient scope"))
		return &res, nil
	}

	id := int64(params.ID)

	scoped := auth.TokenScoped(ctx)
	tpl, err := h.templates.Get(ctx, scoped, id)
	if errors.Is(err, templates.ErrNotFound) {
		res := externalapi.TemplatesGetNotFound(problem(http.StatusNotFound, "template not found"))
		return &res, nil
	}
	if err != nil {
		return nil, err
	}
	res := mapper.EmailTemplateToResource(tpl)
	return &res, nil
}

func (h *Handlers) TemplatesUpdate(ctx context.Context, req *externalapi.UpdateTemplateInput, params externalapi.TemplatesUpdateParams) (externalapi.TemplatesUpdateRes, error) {
	if !auth.HasScope(auth.GetTokenAuth(ctx), "templates:write") {
		res := externalapi.TemplatesUpdateUnauthorized(problem(http.StatusUnauthorized, "insufficient scope"))
		return &res, nil
	}

	id := int64(params.ID)

	scoped := auth.TokenScoped(ctx)
	tpl, err := h.templates.Update(ctx, scoped, id, templates.UpdateInput{
		Name:    convert.StringPtr(req.Name),
		Subject: convert.StringPtr(req.Subject),
		Body:    convert.StringPtr(req.Body),
	})
	if errors.Is(err, templates.ErrNotFound) {
		res := externalapi.TemplatesUpdateNotFound(problem(http.StatusNotFound, "template not found"))
		return &res, nil
	}
	if errors.Is(err, templates.ErrBlankName) {
		res := externalapi.TemplatesUpdateUnprocessableEntity(problem(http.StatusUnprocessableEntity, "name must not be empty"))
		return &res, nil
	}
	if err != nil {
		return nil, err
	}
	res := mapper.EmailTemplateToResource(tpl)
	return &res, nil
}

func (h *Handlers) TemplatesDelete(ctx context.Context, params externalapi.TemplatesDeleteParams) (externalapi.TemplatesDeleteRes, error) {
	if !auth.HasScope(auth.GetTokenAuth(ctx), "templates:write") {
		res := externalapi.TemplatesDeleteUnauthorized(problem(http.StatusUnauthorized, "insufficient scope"))
		return &res, nil
	}

	id := int64(params.ID)

	scoped := auth.TokenScoped(ctx)
	err := h.templates.Delete(ctx, scoped, id)
	if errors.Is(err, templates.ErrNotFound) {
		res := externalapi.TemplatesDeleteNotFound(problem(http.StatusNotFound, "template not found"))
		return &res, nil
	}
	if err != nil {
		return nil, err
	}
	return &externalapi.TemplatesDeleteNoContent{}, nil
}
