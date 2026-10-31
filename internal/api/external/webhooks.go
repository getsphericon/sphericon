package external

import (
	"context"

	"github.com/samber/lo"

	externalapi "github.com/getsphericon/sphericon/gen/external"
	"github.com/getsphericon/sphericon/internal/api/auth"
	"github.com/getsphericon/sphericon/internal/pagination"
	"github.com/getsphericon/sphericon/internal/webhooks"
)

// webhookResource maps the endpoint; the signing secret is deliberately never part of an
// /api response.
func webhookResource(e webhooks.Endpoint) externalapi.WebhookResource {
	return mapper.WebhookToResource(e.WebhookEndpoint)
}

func (h *Handlers) WebhooksList(ctx context.Context, params externalapi.WebhooksListParams) (externalapi.WebhooksListRes, error) {
	page, err := h.webhooks.List(ctx, auth.TokenScoped(ctx), pagination.ParamsOf(params.Page, params.PageSize))
	if err != nil {
		return nil, err
	}
	out := mapper.WebhooksListPage(pagination.WithItems(page, lo.Map(page.Items, func(e webhooks.Endpoint, _ int) externalapi.WebhookResource { return webhookResource(e) })))
	return &out, nil
}

func (h *Handlers) WebhooksCreate(ctx context.Context, req *externalapi.CreateWebhookInput) (externalapi.WebhooksCreateRes, error) {
	// The signing secret is generated server-side and kept sealed; it is shown
	// only in the app, never through /api.
	e, err := h.webhooks.Create(ctx, auth.TokenScoped(ctx), inputs.WebhookCreate(req))
	if err != nil {
		return nil, err
	}
	res := webhookResource(e)
	return &res, nil
}

func (h *Handlers) WebhooksGet(ctx context.Context, params externalapi.WebhooksGetParams) (externalapi.WebhooksGetRes, error) {
	id := int64(params.ID)

	e, err := h.webhooks.Get(ctx, auth.TokenScoped(ctx), id)
	if err != nil {
		return nil, err
	}
	res := webhookResource(e)
	return &res, nil
}

func (h *Handlers) WebhooksUpdate(ctx context.Context, req *externalapi.UpdateWebhookInput, params externalapi.WebhooksUpdateParams) (externalapi.WebhooksUpdateRes, error) {
	id := int64(params.ID)

	e, err := h.webhooks.Update(ctx, auth.TokenScoped(ctx), id, inputs.WebhookUpdate(req))
	if err != nil {
		return nil, err
	}
	res := webhookResource(e)
	return &res, nil
}

func (h *Handlers) WebhooksDelete(ctx context.Context, params externalapi.WebhooksDeleteParams) (externalapi.WebhooksDeleteRes, error) {
	id := int64(params.ID)

	err := h.webhooks.Delete(ctx, auth.TokenScoped(ctx), id)
	if err != nil {
		return nil, err
	}
	return &externalapi.WebhooksDeleteNoContent{}, nil
}
