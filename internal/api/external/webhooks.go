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
	"github.com/getsphericon/sphericon/internal/webhooks"
	"github.com/samber/lo"
)

// webhookResource builds the API resource by hand (not goverter): the signing
// secret is deliberately never part of an /api response, and a nil event filter
// must render as an empty list.
func webhookResource(e webhooks.Endpoint) externalapi.WebhookResource {
	return externalapi.WebhookResource{
		ID:         externalapi.EntityId(e.ID),
		URL:        e.URL,
		EventTypes: lo.Ternary(e.EventTypes == nil, []string{}, e.EventTypes),
		Enabled:    e.Enabled,
		CreatedAt:  externalapi.Timestamp(e.CreatedAt),
		UpdatedAt:  externalapi.Timestamp(e.UpdatedAt),
	}
}

// webhookRuleProblem maps a webhooks rule sentinel to its 422; ok is false for any
// other error.
func webhookRuleProblem(err error) (externalapi.ProblemDetails, bool) {
	switch {
	case errors.Is(err, webhooks.ErrInvalidURL):
		p := problem(http.StatusUnprocessableEntity, "url must be an absolute http or https URL")
		p.Errors = externalapi.NewOptProblemDetailsErrors(externalapi.ProblemDetailsErrors{"url": {"must be an absolute http or https URL"}})
		return p, true
	case errors.Is(err, webhooks.ErrAuditNeedsLicense):
		const detail = "audit.entry needs an Enterprise license"
		p := problem(http.StatusUnprocessableEntity, detail)
		p.Errors = externalapi.NewOptProblemDetailsErrors(externalapi.ProblemDetailsErrors{"eventTypes": {detail}})
		return p, true
	}
	return externalapi.ProblemDetails{}, false
}

func (h *Handlers) WebhooksList(ctx context.Context, params externalapi.WebhooksListParams) (externalapi.WebhooksListRes, error) {
	if !auth.HasScope(auth.GetTokenAuth(ctx), "webhooks:read") {
		res := externalapi.WebhooksListUnauthorized(problem(http.StatusUnauthorized, "insufficient scope"))
		return &res, nil
	}

	page, err := h.webhooks.List(ctx, auth.TokenScoped(ctx), pagination.ParamsOf(params.Page, params.PageSize))
	if err != nil {
		return nil, err
	}
	return &externalapi.WebhooksListOK{
		Items:      lo.Map(page.Items, func(e webhooks.Endpoint, _ int) externalapi.WebhookResource { return webhookResource(e) }),
		Page:       int32(page.Page),
		PageSize:   int32(page.PageSize),
		TotalItems: int32(page.TotalItems),
		TotalPages: int32(page.TotalPages),
	}, nil
}

func (h *Handlers) WebhooksCreate(ctx context.Context, req *externalapi.CreateWebhookInput) (externalapi.WebhooksCreateRes, error) {
	if !auth.HasScope(auth.GetTokenAuth(ctx), "webhooks:write") {
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
	if p, ok := webhookRuleProblem(err); ok {
		res := externalapi.WebhooksCreateUnprocessableEntity(p)
		return &res, nil
	}
	if err != nil {
		return nil, err
	}
	res := webhookResource(e)
	return &res, nil
}

func (h *Handlers) WebhooksGet(ctx context.Context, params externalapi.WebhooksGetParams) (externalapi.WebhooksGetRes, error) {
	if !auth.HasScope(auth.GetTokenAuth(ctx), "webhooks:read") {
		res := externalapi.WebhooksGetUnauthorized(problem(http.StatusUnauthorized, "insufficient scope"))
		return &res, nil
	}

	id := int64(params.ID)

	e, err := h.webhooks.Get(ctx, auth.TokenScoped(ctx), id)
	if ent.IsNotFound(err) {
		res := externalapi.WebhooksGetNotFound(problem(http.StatusNotFound, "webhook not found"))
		return &res, nil
	}
	if err != nil {
		return nil, err
	}
	res := webhookResource(e)
	return &res, nil
}

func (h *Handlers) WebhooksUpdate(ctx context.Context, req *externalapi.UpdateWebhookInput, params externalapi.WebhooksUpdateParams) (externalapi.WebhooksUpdateRes, error) {
	if !auth.HasScope(auth.GetTokenAuth(ctx), "webhooks:write") {
		res := externalapi.WebhooksUpdateUnauthorized(problem(http.StatusUnauthorized, "insufficient scope"))
		return &res, nil
	}

	id := int64(params.ID)

	e, err := h.webhooks.Update(ctx, auth.TokenScoped(ctx), id, webhooks.UpdateInput{
		URL:        convert.StringPtr(req.URL),
		EventTypes: req.EventTypes,
		Enabled:    convert.Ptr(req.Enabled),
	})
	if p, ok := webhookRuleProblem(err); ok {
		res := externalapi.WebhooksUpdateUnprocessableEntity(p)
		return &res, nil
	}
	if ent.IsNotFound(err) {
		res := externalapi.WebhooksUpdateNotFound(problem(http.StatusNotFound, "webhook not found"))
		return &res, nil
	}
	if err != nil {
		return nil, err
	}
	res := webhookResource(e)
	return &res, nil
}

func (h *Handlers) WebhooksDelete(ctx context.Context, params externalapi.WebhooksDeleteParams) (externalapi.WebhooksDeleteRes, error) {
	if !auth.HasScope(auth.GetTokenAuth(ctx), "webhooks:write") {
		res := externalapi.WebhooksDeleteUnauthorized(problem(http.StatusUnauthorized, "insufficient scope"))
		return &res, nil
	}

	id := int64(params.ID)

	err := h.webhooks.Delete(ctx, auth.TokenScoped(ctx), id)
	if ent.IsNotFound(err) {
		res := externalapi.WebhooksDeleteNotFound(problem(http.StatusNotFound, "webhook not found"))
		return &res, nil
	}
	if err != nil {
		return nil, err
	}
	return &externalapi.WebhooksDeleteNoContent{}, nil
}
