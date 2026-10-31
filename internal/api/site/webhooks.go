package site

import (
	"context"

	"github.com/samber/lo"

	siteapi "github.com/getsphericon/sphericon/gen/site"
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
	scoped := workspaceScope(ctx)

	page, err := h.webhooks.List(ctx, scoped, pagination.ParamsOf(params.Page, params.PageSize))
	if err != nil {
		return nil, err
	}
	out := mapper.WebhooksListPage(pagination.WithItems(page, lo.Map(page.Items, func(e webhooks.Endpoint, _ int) siteapi.SiteWebhookEndpointResource { return webhookResource(e) })))
	return &out, nil
}

func (h *Handlers) SiteWebhooksCreate(ctx context.Context, req *siteapi.CreateWebhookInput, _ siteapi.SiteWebhooksCreateParams) (siteapi.SiteWebhooksCreateRes, error) {
	scoped := workspaceScope(ctx)

	e, err := h.webhooks.Create(ctx, scoped, inputs.WebhookCreate(req))
	if err != nil {
		return nil, err
	}
	res := webhookResource(e)
	return &res, nil
}

func (h *Handlers) SiteWebhooksGet(ctx context.Context, params siteapi.SiteWebhooksGetParams) (siteapi.SiteWebhooksGetRes, error) {
	scoped := workspaceScope(ctx)

	id := int64(params.ID)
	e, err := h.webhooks.Get(ctx, scoped, id)
	if err != nil {
		return nil, err
	}
	res := webhookResource(e)
	return &res, nil
}

func (h *Handlers) SiteWebhooksUpdate(ctx context.Context, req *siteapi.UpdateWebhookInput, params siteapi.SiteWebhooksUpdateParams) (siteapi.SiteWebhooksUpdateRes, error) {
	scoped := workspaceScope(ctx)

	id := int64(params.ID)

	e, err := h.webhooks.Update(ctx, scoped, id, inputs.WebhookUpdate(req))
	if err != nil {
		return nil, err
	}
	res := webhookResource(e)
	return &res, nil
}

func (h *Handlers) SiteWebhooksDelete(ctx context.Context, params siteapi.SiteWebhooksDeleteParams) (siteapi.SiteWebhooksDeleteRes, error) {
	scoped := workspaceScope(ctx)

	id := int64(params.ID)
	err := h.webhooks.Delete(ctx, scoped, id)
	if err != nil {
		return nil, err
	}
	return &siteapi.SiteWebhooksDeleteNoContent{}, nil
}
