package external

import (
	"context"

	"github.com/getsphericon/sphericon/ent"

	externalapi "github.com/getsphericon/sphericon/gen/external"
	"github.com/getsphericon/sphericon/internal/api/auth"
	"github.com/getsphericon/sphericon/internal/consent"
	"github.com/getsphericon/sphericon/internal/eligibility"
	"github.com/getsphericon/sphericon/internal/suppression"
)

// Consent narrows through /api (ADR 0016): these operations add a Suppression or record an
// Unsubscribe. It widens only by the evidence of consent the customer collected, recorded as a
// Confirmation (ADR 0077), which never lifts an opt-out. Resubscribing and lifting a Suppression
// are deliberately absent: a human (or the contact via link or START) does them.

func (h *Handlers) SuppressionsCreate(ctx context.Context, req *externalapi.CreateSuppressionInput) (externalapi.SuppressionsCreateRes, error) {
	s := auth.TokenScoped(ctx)

	created, err := suppression.AddManual(ctx, s, inputs.SuppressionChannel(req.Channel.Or(externalapi.SuppressionChannelEmail)), req.Destination)
	if err != nil {
		return nil, err
	}
	res := mapper.SuppressionToResource(created)
	return &res, nil
}

func (h *Handlers) UnsubscribesCreate(ctx context.Context, req *externalapi.CreateUnsubscribeInput) (externalapi.UnsubscribesCreateRes, error) {
	s := auth.TokenScoped(ctx)

	var u *ent.Unsubscribe
	var err error
	switch req.Channel.Or(externalapi.ConsentChannelEmail) {
	case externalapi.ConsentChannelTelegram:
		u, err = consent.TelegramUnsubscribe(ctx, h.bus, s, req.Destination, req.SendingSource.Or(eligibility.SourceEverything))
	case externalapi.ConsentChannelSMS:
		u, err = consent.SMSUnsubscribe(ctx, h.bus, s, req.Destination, req.SendingSource.Or(eligibility.SourceEverything))
	default:
		u, err = consent.Unsubscribe(ctx, h.bus, s, req.Destination, req.SendingSource.Or(eligibility.SourceBroadcasts))
	}
	if err != nil {
		return nil, err
	}
	res := mapper.UnsubscribeToResource(u)
	return &res, nil
}

func (h *Handlers) ConfirmationsCreate(ctx context.Context, req *externalapi.CreateConfirmationInput) (externalapi.ConfirmationsCreateRes, error) {
	c, err := consent.Confirm(ctx, h.bus, auth.TokenScoped(ctx), string(req.Channel), req.Destination, inputs.ConfirmationEvidence(req))
	if err != nil {
		return nil, err
	}
	res := mapper.ConfirmationToResource(c)
	return &res, nil
}
