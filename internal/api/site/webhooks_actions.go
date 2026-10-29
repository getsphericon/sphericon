package site

import (
	"context"
	"net/http"

	siteapi "github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/accounts"
	"github.com/getsphericon/sphericon/internal/narrow"
)

// SiteWebhooksRegenerateSecret replaces the endpoint's signing secret and returns the
// endpoint with the new one.
func (h *Handlers) SiteWebhooksRegenerateSecret(ctx context.Context, params siteapi.SiteWebhooksRegenerateSecretParams) (siteapi.SiteWebhooksRegenerateSecretRes, error) {
	scoped, role, err := h.scopedWithRoleFor(ctx, params.WorkspaceId)
	if err != nil {
		return nil, err
	}
	if !accounts.CanManageWebhooks(role) {
		v := siteapi.SiteWebhooksRegenerateSecretForbidden(problem(http.StatusForbidden, errWebhooksForbidden))
		return &v, nil
	}
	e, err := h.webhooks.RegenerateSecret(ctx, scoped, int64(params.ID))
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
	if err != nil {
		return nil, err
	}
	if !accounts.CanManageWebhooks(role) {
		v := siteapi.SiteWebhooksTestForbidden(problem(http.StatusForbidden, errWebhooksForbidden))
		return &v, nil
	}
	res, err := h.webhooks.TestSend(ctx, scoped, int64(params.ID), req.EventType)
	if err != nil {
		return nil, err
	}
	out := siteapi.SiteWebhookTestResult{Delivered: res.Delivered}
	if res.Status != 0 {
		out.Status = siteapi.NewOptInt32(narrow.Int32(res.Status))
	}
	if res.Kind != "" {
		out.Kind = siteapi.NewOptSiteWebhookErrorKind(siteapi.SiteWebhookErrorKind(res.Kind))
	}
	return &out, nil
}
