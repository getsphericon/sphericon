package external

import (
	"context"

	externalapi "github.com/getsphericon/sphericon/gen/external"
	"github.com/getsphericon/sphericon/internal/api/auth"
	"github.com/getsphericon/sphericon/internal/consent"
	"github.com/getsphericon/sphericon/internal/suppression"
)

// Consent narrows through /api (ADR 0016): these operations add a Suppression or record an
// Unsubscribe. It widens only by the evidence of consent the customer collected, recorded as a
// Confirmation (ADR 0077), which never lifts an opt-out. Resubscribing and lifting a Suppression
// are deliberately absent: a human (or the contact via link or START) does them.

func (h *Handlers) SuppressionsCreate(ctx context.Context, req *externalapi.CreateSuppressionInput) (*externalapi.SuppressionResource, error) {
	s := auth.TokenScoped(ctx)

	created, err := suppression.AddManual(ctx, s, inputs.SuppressionChannel(req.Channel.Or(externalapi.SuppressionChannelEmail)), req.Destination)
	if err != nil {
		return nil, err
	}
	res := mapper.SuppressionToResource(created)
	return &res, nil
}

func (h *Handlers) UnsubscribesCreate(ctx context.Context, req *externalapi.CreateUnsubscribeInput) (*externalapi.UnsubscribeResource, error) {
	channel := inputs.ConsentChannel(req.Channel.Or(externalapi.ConsentChannelEmail))
	u, err := consent.OptOut(ctx, h.bus, auth.TokenScoped(ctx), channel, req.Destination, req.SendingSource.Or(""))
	if err != nil {
		return nil, err
	}
	res := mapper.UnsubscribeToResource(u)
	return &res, nil
}

func (h *Handlers) ConfirmationsCreate(ctx context.Context, req *externalapi.CreateConfirmationInput) (*externalapi.ConfirmationResource, error) {
	c, err := consent.Confirm(ctx, h.bus, auth.TokenScoped(ctx), inputs.ConsentChannel(req.Channel), req.Destination, inputs.ConfirmationEvidence(req))
	if err != nil {
		return nil, err
	}
	res := mapper.ConfirmationToResource(c)
	return &res, nil
}
