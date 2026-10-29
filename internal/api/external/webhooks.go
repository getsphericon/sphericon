package external

import (
	"context"
	"net/http"

	externalapi "github.com/getsphericon/sphericon/gen/external"
	"github.com/getsphericon/sphericon/internal/api/auth"
	"github.com/getsphericon/sphericon/internal/convert"
	"github.com/getsphericon/sphericon/internal/narrow"
	"github.com/getsphericon/sphericon/internal/pagination"
	"github.com/getsphericon/sphericon/internal/webhooks"
	"github.com/samber/lo"
)

// webhookResource maps the endpoint; the signing secret is deliberately never part of an
// /api response.
func webhookResource(e webhooks.Endpoint) externalapi.WebhookResource {
	return mapper.WebhookToResource(e.WebhookEndpoint)
}

func (h *Handlers) WebhooksList(ctx context.Context, params externalapi.WebhooksListParams) (externalapi.WebhooksListRes, error) {
	if !auth.HasScope(auth.GetTokenAuth(ctx), externalapi.ApiTokenScopeWebhooksRead) {
		res := externalapi.WebhooksListUnauthorized(problem(http.StatusUnauthorized, "insufficient scope"))
		return &res, nil
	}

	page, err := h.webhooks.List(ctx, auth.TokenScoped(ctx), pagination.ParamsOf(params.Page, params.PageSize))
	if err != nil {
		return nil, err
	}
	return &externalapi.WebhooksListOK{
		Items:      lo.Map(page.Items, func(e webhooks.Endpoint, _ int) externalapi.WebhookResource { return webhookResource(e) }),
		Page:       narrow.Int32(page.Page),
		PageSize:   narrow.Int32(page.PageSize),
		TotalItems: narrow.Int32(page.TotalItems),
		TotalPages: narrow.Int32(page.TotalPages),
	}, nil
}

func (h *Handlers) WebhooksCreate(ctx context.Context, req *externalapi.CreateWebhookInput) (externalapi.WebhooksCreateRes, error) {
	if !auth.HasScope(auth.GetTokenAuth(ctx), externalapi.ApiTokenScopeWebhooksWrite) {
		res := externalapi.WebhooksCreateUnauthorized(problem(http.StatusUnauthorized, "insufficient scope"))
		return &res, nil
	}

	// The signing secret is generated server-side and kept sealed; it is shown
	// only in the app, never through /api.
	e, err := h.webhooks.Create(ctx, auth.TokenScoped(ctx), webhooks.CreateInput{
		URL:        req.URL,
		EventTypes: req.EventTypes,
		Enabled:    convert.Ptr(req.Enabled),
	})
	if err != nil {
		return nil, err
	}
	res := webhookResource(e)
	return &res, nil
}

func (h *Handlers) WebhooksGet(ctx context.Context, params externalapi.WebhooksGetParams) (externalapi.WebhooksGetRes, error) {
	if !auth.HasScope(auth.GetTokenAuth(ctx), externalapi.ApiTokenScopeWebhooksRead) {
		res := externalapi.WebhooksGetUnauthorized(problem(http.StatusUnauthorized, "insufficient scope"))
		return &res, nil
	}

	id := int64(params.ID)

	e, err := h.webhooks.Get(ctx, auth.TokenScoped(ctx), id)
	if err != nil {
		return nil, err
	}
	res := webhookResource(e)
	return &res, nil
}

func (h *Handlers) WebhooksUpdate(ctx context.Context, req *externalapi.UpdateWebhookInput, params externalapi.WebhooksUpdateParams) (externalapi.WebhooksUpdateRes, error) {
	if !auth.HasScope(auth.GetTokenAuth(ctx), externalapi.ApiTokenScopeWebhooksWrite) {
		res := externalapi.WebhooksUpdateUnauthorized(problem(http.StatusUnauthorized, "insufficient scope"))
		return &res, nil
	}

	id := int64(params.ID)

	e, err := h.webhooks.Update(ctx, auth.TokenScoped(ctx), id, webhooks.UpdateInput{
		URL:        convert.StringPtr(req.URL),
		EventTypes: req.EventTypes,
		Enabled:    convert.Ptr(req.Enabled),
	})
	if err != nil {
		return nil, err
	}
	res := webhookResource(e)
	return &res, nil
}

func (h *Handlers) WebhooksDelete(ctx context.Context, params externalapi.WebhooksDeleteParams) (externalapi.WebhooksDeleteRes, error) {
	if !auth.HasScope(auth.GetTokenAuth(ctx), externalapi.ApiTokenScopeWebhooksWrite) {
		res := externalapi.WebhooksDeleteUnauthorized(problem(http.StatusUnauthorized, "insufficient scope"))
		return &res, nil
	}

	id := int64(params.ID)

	err := h.webhooks.Delete(ctx, auth.TokenScoped(ctx), id)
	if err != nil {
		return nil, err
	}
	return &externalapi.WebhooksDeleteNoContent{}, nil
}
