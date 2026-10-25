package site

import (
	"context"
	"net/http"

	siteapi "github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/erasure"
	"github.com/getsphericon/sphericon/internal/narrow"
)

// SiteContactsLookup answers a Privacy request's first question (ADR 0021): what the
// Workspace holds for an email address, as counts. It sits beside Erasure, so owner
// or admin only; an address nothing holds answers with zeros.
func (h *Handlers) SiteContactsLookup(ctx context.Context, params siteapi.SiteContactsLookupParams) (siteapi.SiteContactsLookupRes, error) {
	scoped, refusal, err := h.privacyScope(ctx, params.WorkspaceId, params.Email)
	if err != nil {
		return nil, err
	}
	if refusal != nil {
		switch refusal.Status.Value {
		case http.StatusNotFound:
			v := siteapi.SiteContactsLookupNotFound(*refusal)
			return &v, nil
		case http.StatusForbidden:
			v := siteapi.SiteContactsLookupForbidden(*refusal)
			return &v, nil
		default:
			v := siteapi.SiteContactsLookupBadRequest(*refusal)
			return &v, nil
		}
	}

	held, err := erasure.Lookup(ctx, scoped, params.Email)
	if err != nil {
		return nil, err
	}
	return &siteapi.SiteContactHoldings{
		ContactHeld:      held.ContactHeld,
		Events:           narrow.Int32(held.Events),
		DeliveryRecords:  narrow.Int32(held.DeliveryRecords),
		Unsubscribes:     narrow.Int32(held.Unsubscribes),
		Suppressions:     narrow.Int32(held.Suppressions),
		CapturedMessages: narrow.Int32(held.CapturedMessages),
	}, nil
}
