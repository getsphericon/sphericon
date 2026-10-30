package site

import (
	"context"

	"github.com/samber/lo"

	siteapi "github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/accounts"
	"github.com/getsphericon/sphericon/internal/narrow"
	"github.com/getsphericon/sphericon/internal/pagination"
	"github.com/getsphericon/sphericon/internal/webhooks"
)

// webhookResource maps the endpoint with its decrypted signing secret, which the
// UI displays for signature verification.
func webhookResource(e webhooks.Endpoint) siteapi.SiteWebhookEndpointResource {
	res := mapper.WebhookEndpointToResource(e.WebhookEndpoint)
	res.Secret = e.Secret
	return res
}

func (h *Handlers) SiteWebhooksList(ctx context.Context, params siteapi.SiteWebhooksListParams) (siteapi.SiteWebhooksListRes, error) {
	scoped, role, err := h.scopedWithRoleFor(ctx, params.WorkspaceId)
	if err != nil {
		return nil, err
	}
	if err := accounts.Require(role, accounts.PermWebhooksManage); err != nil {
		return nil, err
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
	if err := accounts.Require(role, accounts.PermWebhooksManage); err != nil {
		return nil, err
	}

	enabled, hasEnabled := req.Enabled.Get()
	e, err := h.webhooks.Create(ctx, scoped, webhooks.CreateInput{
		URL:        req.URL,
		EventTypes: req.EventTypes,
		Enabled:    lo.Ternary(hasEnabled, &enabled, nil),
	})
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
	if err := accounts.Require(role, accounts.PermWebhooksManage); err != nil {
		return nil, err
	}

	id := int64(params.ID)
	e, err := h.webhooks.Get(ctx, scoped, id)
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
	if err := accounts.Require(role, accounts.PermWebhooksManage); err != nil {
		return nil, err
	}

	id := int64(params.ID)

	url, hasURL := req.URL.Get()
	enabled, hasEnabled := req.Enabled.Get()
	e, err := h.webhooks.Update(ctx, scoped, id, webhooks.UpdateInput{
		URL:        lo.Ternary(hasURL, &url, nil),
		EventTypes: req.EventTypes,
		Enabled:    lo.Ternary(hasEnabled, &enabled, nil),
	})
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
	if err := accounts.Require(role, accounts.PermWebhooksManage); err != nil {
		return nil, err
	}

	id := int64(params.ID)
	err = h.webhooks.Delete(ctx, scoped, id)
	if err != nil {
		return nil, err
	}
	return &siteapi.SiteWebhooksDeleteNoContent{}, nil
}
