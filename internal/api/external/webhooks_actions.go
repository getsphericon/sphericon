package external

import (
	"context"
	"errors"
	"net/http"

	"github.com/getsphericon/sphericon/ent"
	externalapi "github.com/getsphericon/sphericon/gen/external"
	"github.com/getsphericon/sphericon/internal/api/auth"
	"github.com/getsphericon/sphericon/internal/webhooks"
)

// WebhooksRegenerateSecret replaces the endpoint's signing secret. The new secret is
// not part of the response: the external API never returns it.
func (h *Handlers) WebhooksRegenerateSecret(ctx context.Context, params externalapi.WebhooksRegenerateSecretParams) (externalapi.WebhooksRegenerateSecretRes, error) {
	if !auth.HasScope(auth.GetTokenAuth(ctx), externalapi.ApiTokenScopeWebhooksRegenerate) {
		res := externalapi.WebhooksRegenerateSecretUnauthorized(problem(http.StatusUnauthorized, "insufficient scope"))
		return &res, nil
	}
	e, err := h.webhooks.RegenerateSecret(ctx, auth.TokenScoped(ctx), int64(params.ID))
	if ent.IsNotFound(err) {
		res := externalapi.WebhooksRegenerateSecretNotFound(problem(http.StatusNotFound, "webhook not found"))
		return &res, nil
	}
	if err != nil {
		return nil, err
	}
	res := webhookResource(e)
	return &res, nil
}

// WebhooksTest sends a sample event to the endpoint and returns what the receiver
// answered; the endpoint's failure state is not touched.
func (h *Handlers) WebhooksTest(ctx context.Context, req *externalapi.TestWebhookInput, params externalapi.WebhooksTestParams) (externalapi.WebhooksTestRes, error) {
	if !auth.HasScope(auth.GetTokenAuth(ctx), externalapi.ApiTokenScopeWebhooksTest) {
		res := externalapi.WebhooksTestUnauthorized(problem(http.StatusUnauthorized, "insufficient scope"))
		return &res, nil
	}
	res, err := h.webhooks.TestSend(ctx, auth.TokenScoped(ctx), int64(params.ID), req.EventType)
	if errors.Is(err, webhooks.ErrInvalidEventType) {
		p := problem(http.StatusUnprocessableEntity, err.Error())
		p.Errors = externalapi.NewOptProblemDetailsErrors(externalapi.ProblemDetailsErrors{"eventType": {err.Error()}})
		v := externalapi.WebhooksTestUnprocessableEntity(p)
		return &v, nil
	}
	if ent.IsNotFound(err) {
		v := externalapi.WebhooksTestNotFound(problem(http.StatusNotFound, "webhook not found"))
		return &v, nil
	}
	if err != nil {
		return nil, err
	}
	out := externalapi.WebhookTestResult{Delivered: res.Delivered}
	if res.Status != 0 {
		out.Status = externalapi.NewOptInt32(int32(res.Status))
	}
	if res.Kind != "" {
		out.Kind = externalapi.NewOptWebhookErrorKind(externalapi.WebhookErrorKind(res.Kind))
	}
	return &out, nil
}
