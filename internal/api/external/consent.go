package external

import (
	"context"
	"errors"
	"net/http"

	externalapi "github.com/getsphericon/sphericon/gen/external"
	"github.com/getsphericon/sphericon/internal/api/auth"
	"github.com/getsphericon/sphericon/internal/consent"
	"github.com/getsphericon/sphericon/internal/eligibility"
	"github.com/getsphericon/sphericon/internal/suppression"
)

// Consent only narrows through /api (ADR 0016): these two operations add a
// Suppression or record an Unsubscribe. Resubscribing and lifting a Suppression are
// deliberately absent — they widen reach and assert consent, so a human (or the
// contact via link) does them.

func (h *Handlers) SuppressionsCreate(ctx context.Context, req *externalapi.CreateSuppressionInput) (externalapi.SuppressionsCreateRes, error) {
	if !auth.HasScope(auth.GetTokenAuth(ctx), externalapi.ApiTokenScopeContactsWrite) {
		res := externalapi.SuppressionsCreateUnauthorized(problem(http.StatusUnauthorized, "insufficient scope"))
		return &res, nil
	}
	s := auth.TokenScoped(ctx)

	created, err := suppression.AddManual(ctx, s, string(req.Destination))
	if errors.Is(err, suppression.ErrDestinationEmpty) {
		res := externalapi.SuppressionsCreateUnprocessableEntity(problem(http.StatusUnprocessableEntity, "destination must not be empty"))
		return &res, nil
	}
	if err != nil {
		return nil, err
	}
	res := mapper.SuppressionToResource(created)
	return &res, nil
}

func (h *Handlers) UnsubscribesCreate(ctx context.Context, req *externalapi.CreateUnsubscribeInput) (externalapi.UnsubscribesCreateRes, error) {
	if !auth.HasScope(auth.GetTokenAuth(ctx), externalapi.ApiTokenScopeContactsWrite) {
		res := externalapi.UnsubscribesCreateUnauthorized(problem(http.StatusUnauthorized, "insufficient scope"))
		return &res, nil
	}
	s := auth.TokenScoped(ctx)

	source := req.SendingSource.Or(eligibility.SourceBroadcasts)
	u, err := consent.Unsubscribe(ctx, h.bus, s, string(req.Destination), source)
	switch {
	case errors.Is(err, consent.ErrDestinationEmpty), errors.Is(err, consent.ErrInvalidSource), errors.Is(err, consent.ErrUnknownAutomation):
		res := externalapi.UnsubscribesCreateUnprocessableEntity(problem(http.StatusUnprocessableEntity, err.Error()))
		return &res, nil
	case err != nil:
		return nil, err
	}
	res := mapper.UnsubscribeToResource(u)
	return &res, nil
}
