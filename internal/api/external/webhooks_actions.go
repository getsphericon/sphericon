package external

import (
	"context"

	externalapi "github.com/getsphericon/sphericon/gen/external"
	"github.com/getsphericon/sphericon/internal/api/auth"
	"github.com/getsphericon/sphericon/internal/narrow"
)

// WebhooksRegenerateSecret replaces the endpoint's signing secret. The new secret is
// not part of the response: the external API never returns it.
func (h *Handlers) WebhooksRegenerateSecret(ctx context.Context, params externalapi.WebhooksRegenerateSecretParams) (externalapi.WebhooksRegenerateSecretRes, error) {
	e, err := h.webhooks.RegenerateSecret(ctx, auth.TokenScoped(ctx), int64(params.ID))
	if err != nil {
		return nil, err
	}
	res := webhookResource(e)
	return &res, nil
}

// WebhooksTest sends a sample event to the endpoint and returns what the receiver
// answered; the endpoint's failure state is not touched.
func (h *Handlers) WebhooksTest(ctx context.Context, req *externalapi.TestWebhookInput, params externalapi.WebhooksTestParams) (externalapi.WebhooksTestRes, error) {
	res, err := h.webhooks.TestSend(ctx, auth.TokenScoped(ctx), int64(params.ID), req.EventType)
	if err != nil {
		return nil, err
	}
	out := externalapi.WebhookTestResult{Delivered: res.Delivered}
	if res.Status != 0 {
		out.Status = externalapi.NewOptInt32(narrow.Int32(res.Status))
	}
	if res.Kind != "" {
		out.Kind = externalapi.NewOptWebhookErrorKind(mapper.DeliveryErrorKindToResource(res.Kind))
	}
	return &out, nil
}
