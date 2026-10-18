package site

import (
	"context"
	"errors"
	"net/http"

	"github.com/samber/lo"

	"github.com/getsphericon/sphericon/ent"
	siteapi "github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/i18n"
	"github.com/getsphericon/sphericon/internal/pagination"
	"github.com/getsphericon/sphericon/internal/webhooks"
)

// webhookResource maps the endpoint with its decrypted signing secret, which the
// UI displays for signature verification.
func webhookResource(e webhooks.Endpoint) siteapi.SiteWebhookEndpointResource {
	return siteapi.SiteWebhookEndpointResource{
		ID:         siteapi.EntityId(e.ID),
		URL:        e.URL,
		Secret:     e.Secret,
		EventTypes: lo.Ternary(e.EventTypes == nil, []string{}, e.EventTypes),
		Enabled:    e.Enabled,
		Failure:    webhookFailure(e),
		CreatedAt:  siteapi.Timestamp(e.CreatedAt),
		UpdatedAt:  siteapi.Timestamp(e.UpdatedAt),
	}
}

// webhookFailure maps the endpoint's failure state; it is absent while the endpoint is healthy.
func webhookFailure(e webhooks.Endpoint) siteapi.OptSiteWebhookFailure {
	if e.ConsecutiveFailures == nil || e.FailingSince == nil || e.LastFailureAt == nil || e.LastErrorKind == nil {
		return siteapi.OptSiteWebhookFailure{}
	}
	f := siteapi.SiteWebhookFailure{
		ConsecutiveFailures: int32(*e.ConsecutiveFailures),
		Since:               siteapi.Timestamp(*e.FailingSince),
		LastFailureAt:       siteapi.Timestamp(*e.LastFailureAt),
		Kind:                siteapi.SiteWebhookErrorKind(*e.LastErrorKind),
	}
	if e.LastErrorStatus != nil {
		f.Status = siteapi.NewOptInt32(int32(*e.LastErrorStatus))
	}
	return siteapi.NewOptSiteWebhookFailure(f)
}

// webhookRuleProblem maps a webhooks rule sentinel to its 422; ok is false for any
// other error.
func webhookRuleProblem(err error) (siteapi.ProblemDetails, bool) {
	switch {
	case errors.Is(err, webhooks.ErrInvalidURL):
		return problemWithErrors(http.StatusUnprocessableEntity, i18n.T("errors.url_invalid", nil), map[string][]string{
			"url": {i18n.T("errors.url_must_be_absolute", nil)},
		}), true
	case errors.Is(err, webhooks.ErrAuditNeedsLicense):
		const detail = "audit.entry needs an Enterprise license"
		return problemWithErrors(http.StatusUnprocessableEntity, detail, map[string][]string{"eventTypes": {detail}}), true
	}
	return siteapi.ProblemDetails{}, false
}

func (h *Handlers) SiteWebhooksList(ctx context.Context, params siteapi.SiteWebhooksListParams) (siteapi.SiteWebhooksListRes, error) {
	scoped, err := h.scopedFor(ctx, params.Slug)
	if ent.IsNotFound(err) {
		v := siteapi.SiteWebhooksListNotFound(problem(http.StatusNotFound, "workspace not found"))
		return &v, nil
	}
	if err != nil {
		return nil, err
	}

	page, err := h.webhooks.List(ctx, scoped, pagination.ParamsOf(params.Page, params.PageSize))
	if err != nil {
		return nil, err
	}
	return &siteapi.SiteWebhooksListOK{
		Items:      lo.Map(page.Items, func(e webhooks.Endpoint, _ int) siteapi.SiteWebhookEndpointResource { return webhookResource(e) }),
		Page:       int32(page.Page),
		PageSize:   int32(page.PageSize),
		TotalItems: int32(page.TotalItems),
		TotalPages: int32(page.TotalPages),
	}, nil
}

func (h *Handlers) SiteWebhooksCreate(ctx context.Context, req *siteapi.SiteCreateWebhookEndpointInput, params siteapi.SiteWebhooksCreateParams) (siteapi.SiteWebhooksCreateRes, error) {
	scoped, err := h.scopedFor(ctx, params.Slug)
	if ent.IsNotFound(err) {
		v := siteapi.SiteWebhooksCreateNotFound(problem(http.StatusNotFound, "workspace not found"))
		return &v, nil
	}
	if err != nil {
		return nil, err
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
	scoped, err := h.scopedFor(ctx, params.Slug)
	if ent.IsNotFound(err) {
		v := siteapi.SiteWebhooksGetNotFound(problem(http.StatusNotFound, "workspace not found"))
		return &v, nil
	}
	if err != nil {
		return nil, err
	}

	id := int64(params.ID)
	e, err := h.webhooks.Get(ctx, scoped, id)
	if ent.IsNotFound(err) {
		v := siteapi.SiteWebhooksGetNotFound(problem(http.StatusNotFound, "webhook not found"))
		return &v, nil
	}
	if err != nil {
		return nil, err
	}
	res := webhookResource(e)
	return &res, nil
}

func (h *Handlers) SiteWebhooksUpdate(ctx context.Context, req *siteapi.SiteUpdateWebhookEndpointInput, params siteapi.SiteWebhooksUpdateParams) (siteapi.SiteWebhooksUpdateRes, error) {
	scoped, err := h.scopedFor(ctx, params.Slug)
	if ent.IsNotFound(err) {
		v := siteapi.SiteWebhooksUpdateNotFound(problem(http.StatusNotFound, "workspace not found"))
		return &v, nil
	}
	if err != nil {
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
	if p, ok := webhookRuleProblem(err); ok {
		v := siteapi.SiteWebhooksUpdateUnprocessableEntity(p)
		return &v, nil
	}
	if ent.IsNotFound(err) {
		v := siteapi.SiteWebhooksUpdateNotFound(problem(http.StatusNotFound, "webhook not found"))
		return &v, nil
	}
	if err != nil {
		return nil, err
	}
	res := webhookResource(e)
	return &res, nil
}

func (h *Handlers) SiteWebhooksDelete(ctx context.Context, params siteapi.SiteWebhooksDeleteParams) (siteapi.SiteWebhooksDeleteRes, error) {
	scoped, err := h.scopedFor(ctx, params.Slug)
	if ent.IsNotFound(err) {
		v := siteapi.SiteWebhooksDeleteNotFound(problem(http.StatusNotFound, "workspace not found"))
		return &v, nil
	}
	if err != nil {
		return nil, err
	}

	id := int64(params.ID)
	err = h.webhooks.Delete(ctx, scoped, id)
	if ent.IsNotFound(err) {
		v := siteapi.SiteWebhooksDeleteNotFound(problem(http.StatusNotFound, "webhook not found"))
		return &v, nil
	}
	if err != nil {
		return nil, err
	}
	return &siteapi.SiteWebhooksDeleteNoContent{}, nil
}
