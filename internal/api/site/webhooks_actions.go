package site

import (
	"context"
	"errors"
	"net/http"

	"github.com/getsphericon/sphericon/ent"
	siteapi "github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/accounts"
	"github.com/getsphericon/sphericon/internal/webhooks"
)

// SiteWebhooksRegenerateSecret replaces the endpoint's signing secret and returns the
// endpoint with the new one.
func (h *Handlers) SiteWebhooksRegenerateSecret(ctx context.Context, params siteapi.SiteWebhooksRegenerateSecretParams) (siteapi.SiteWebhooksRegenerateSecretRes, error) {
	scoped, role, err := h.scopedWithRoleFor(ctx, params.WorkspaceId)
	if ent.IsNotFound(err) {
		v := siteapi.SiteWebhooksRegenerateSecretNotFound(problem(http.StatusNotFound, "workspace not found"))
		return &v, nil
	}
	if err != nil {
		return nil, err
	}
	if !accounts.CanManageWebhooks(role) {
		v := siteapi.SiteWebhooksRegenerateSecretForbidden(problem(http.StatusForbidden, errWebhooksForbidden))
		return &v, nil
	}
	e, err := h.webhooks.RegenerateSecret(ctx, scoped, int64(params.ID))
	if ent.IsNotFound(err) {
		v := siteapi.SiteWebhooksRegenerateSecretNotFound(problem(http.StatusNotFound, "webhook not found"))
		return &v, nil
	}
	if err != nil {
		return nil, err
	}
	res := webhookResource(e)
	return &res, nil
}

// SiteWebhooksTest sends a sample event to the endpoint and returns what the receiver
// answered, at once.
func (h *Handlers) SiteWebhooksTest(ctx context.Context, req *siteapi.SiteTestWebhookInput, params siteapi.SiteWebhooksTestParams) (siteapi.SiteWebhooksTestRes, error) {
	scoped, role, err := h.scopedWithRoleFor(ctx, params.WorkspaceId)
	if ent.IsNotFound(err) {
		v := siteapi.SiteWebhooksTestNotFound(problem(http.StatusNotFound, "workspace not found"))
		return &v, nil
	}
	if err != nil {
		return nil, err
	}
	if !accounts.CanManageWebhooks(role) {
		v := siteapi.SiteWebhooksTestForbidden(problem(http.StatusForbidden, errWebhooksForbidden))
		return &v, nil
	}
	res, err := h.webhooks.TestSend(ctx, scoped, int64(params.ID), req.EventType)
	if errors.Is(err, webhooks.ErrInvalidEventType) {
		p := problem(http.StatusUnprocessableEntity, err.Error())
		p.Errors = siteapi.NewOptProblemDetailsErrors(siteapi.ProblemDetailsErrors{"eventType": {err.Error()}})
		v := siteapi.SiteWebhooksTestUnprocessableEntity(p)
		return &v, nil
	}
	if ent.IsNotFound(err) {
		v := siteapi.SiteWebhooksTestNotFound(problem(http.StatusNotFound, "webhook not found"))
		return &v, nil
	}
	if err != nil {
		return nil, err
	}
	out := siteapi.SiteWebhookTestResult{Delivered: res.Delivered}
	if res.Status != 0 {
		out.Status = siteapi.NewOptInt32(int32(res.Status))
	}
	if res.Kind != "" {
		out.Kind = siteapi.NewOptSiteWebhookErrorKind(siteapi.SiteWebhookErrorKind(res.Kind))
	}
	return &out, nil
}
