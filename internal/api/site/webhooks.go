package site

import (
	"context"
	"errors"
	"net/http"

	"github.com/samber/lo"

	siteapi "github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/accounts"
	"github.com/getsphericon/sphericon/internal/narrow"
	"github.com/getsphericon/sphericon/internal/pagination"
	"github.com/getsphericon/sphericon/internal/validation"
	"github.com/getsphericon/sphericon/internal/webhooks"
)

const errWebhooksForbidden = "only owners and admins can manage webhooks"

// webhookResource maps the endpoint with its decrypted signing secret, which the
// UI displays for signature verification.
func webhookResource(e webhooks.Endpoint) siteapi.SiteWebhookEndpointResource {
	res := mapper.WebhookEndpointToResource(e.WebhookEndpoint)
	res.Secret = e.Secret
	return res
}

// webhookRuleProblem maps a webhooks rule sentinel to its 422; ok is false for any
// other error.
func webhookRuleProblem(err error) (siteapi.ProblemDetails, bool) {
	switch {
	case errors.Is(err, webhooks.ErrInvalidURL):
		return fieldProblem(err, "url"), true
	case errors.Is(err, webhooks.ErrAuditNeedsLicense):
		return fieldProblem(err, "eventTypes"), true
	}
	return siteapi.ProblemDetails{}, false
}

func (h *Handlers) SiteWebhooksList(ctx context.Context, params siteapi.SiteWebhooksListParams) (siteapi.SiteWebhooksListRes, error) {
	scoped, role, err := h.scopedWithRoleFor(ctx, params.WorkspaceId)
	if err != nil {
		return nil, err
	}
	if !accounts.CanManageWebhooks(role) {
		v := siteapi.SiteWebhooksListForbidden(problem(http.StatusForbidden, errWebhooksForbidden))
		return &v, nil
	}

	page, err := h.webhooks.List(ctx, scoped, pagination.ParamsOf(params.Page, params.PageSize))
	if err != nil {
		return nil, err
	}
	return &siteapi.SiteWebhooksListOK{
		Items:      lo.Map(page.Items, func(e webhooks.Endpoint, _ int) siteapi.SiteWebhookEndpointResource { return webhookResource(e) }),
		Page:       narrow.Int32(page.Page),
		PageSize:   narrow.Int32(page.PageSize),
		TotalItems: narrow.Int32(page.TotalItems),
		TotalPages: narrow.Int32(page.TotalPages),
	}, nil
}

func (h *Handlers) SiteWebhooksCreate(ctx context.Context, req *siteapi.SiteCreateWebhookEndpointInput, params siteapi.SiteWebhooksCreateParams) (siteapi.SiteWebhooksCreateRes, error) {
	scoped, role, err := h.scopedWithRoleFor(ctx, params.WorkspaceId)
	if err != nil {
		return nil, err
	}
	if !accounts.CanManageWebhooks(role) {
		v := siteapi.SiteWebhooksCreateForbidden(problem(http.StatusForbidden, errWebhooksForbidden))
		return &v, nil
	}

	enabled, hasEnabled := req.Enabled.Get()
	e, err := h.webhooks.Create(ctx, scoped, webhooks.CreateInput{
		URL:        req.URL,
		EventTypes: req.EventTypes,
		Enabled:    lo.Ternary(hasEnabled, &enabled, nil),
	})
	if p, ok := webhookRuleProblem(err); ok {
		v := siteapi.SiteWebhooksCreateUnprocessableEntity(p)
		return &v, nil
	}
	if err != nil {
		return nil, err
	}
	res := webhookResource(e)
	return &res, nil
}

func (h *Handlers) SiteWebhooksGet(ctx context.Context, params siteapi.SiteWebhooksGetParams) (siteapi.SiteWebhooksGetRes, error) {
	scoped, role, err := h.scopedWithRoleFor(ctx, params.WorkspaceId)
	if err != nil {
		return nil, err
	}
	if !accounts.CanManageWebhooks(role) {
		v := siteapi.SiteWebhooksGetForbidden(problem(http.StatusForbidden, errWebhooksForbidden))
		return &v, nil
	}

	id := int64(params.ID)
	e, err := h.webhooks.Get(ctx, scoped, id)
	if errors.Is(err, webhooks.ErrNotFound) {
		v := siteapi.SiteWebhooksGetNotFound(problem(http.StatusNotFound, validation.Message(err)))
		return &v, nil
	}
	if err != nil {
		return nil, err
	}
	res := webhookResource(e)
	return &res, nil
}

func (h *Handlers) SiteWebhooksUpdate(ctx context.Context, req *siteapi.SiteUpdateWebhookEndpointInput, params siteapi.SiteWebhooksUpdateParams) (siteapi.SiteWebhooksUpdateRes, error) {
	scoped, role, err := h.scopedWithRoleFor(ctx, params.WorkspaceId)
	if err != nil {
		return nil, err
	}
	if !accounts.CanManageWebhooks(role) {
		v := siteapi.SiteWebhooksUpdateForbidden(problem(http.StatusForbidden, errWebhooksForbidden))
		return &v, nil
	}

	id := int64(params.ID)

	url, hasURL := req.URL.Get()
	enabled, hasEnabled := req.Enabled.Get()
	e, err := h.webhooks.Update(ctx, scoped, id, webhooks.UpdateInput{
		URL:        lo.Ternary(hasURL, &url, nil),
		EventTypes: req.EventTypes,
		Enabled:    lo.Ternary(hasEnabled, &enabled, nil),
	})
	if p, ok := webhookRuleProblem(err); ok {
		v := siteapi.SiteWebhooksUpdateUnprocessableEntity(p)
		return &v, nil
	}
	if errors.Is(err, webhooks.ErrNotFound) {
		v := siteapi.SiteWebhooksUpdateNotFound(problem(http.StatusNotFound, validation.Message(err)))
		return &v, nil
	}
	if err != nil {
		return nil, err
	}
	res := webhookResource(e)
	return &res, nil
}

func (h *Handlers) SiteWebhooksDelete(ctx context.Context, params siteapi.SiteWebhooksDeleteParams) (siteapi.SiteWebhooksDeleteRes, error) {
	scoped, role, err := h.scopedWithRoleFor(ctx, params.WorkspaceId)
	if err != nil {
		return nil, err
	}
	if !accounts.CanManageWebhooks(role) {
		v := siteapi.SiteWebhooksDeleteForbidden(problem(http.StatusForbidden, errWebhooksForbidden))
		return &v, nil
	}

	id := int64(params.ID)
	err = h.webhooks.Delete(ctx, scoped, id)
	if errors.Is(err, webhooks.ErrNotFound) {
		v := siteapi.SiteWebhooksDeleteNotFound(problem(http.StatusNotFound, validation.Message(err)))
		return &v, nil
	}
	if err != nil {
		return nil, err
	}
	return &siteapi.SiteWebhooksDeleteNoContent{}, nil
}
