package site

import (
	"context"
	"errors"
	"net/http"

	"github.com/getsphericon/sphericon/ent"
	siteapi "github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/accounts"
	"github.com/getsphericon/sphericon/internal/erasure"
)

// SiteContactsLookup answers a Privacy request's first question (ADR 0021): what the
// Workspace holds for an email address, as counts. It sits beside Erasure, so owner
// or admin only; an address nothing holds answers with zeros.
func (h *Handlers) SiteContactsLookup(ctx context.Context, params siteapi.SiteContactsLookupParams) (siteapi.SiteContactsLookupRes, error) {
	scoped, role, err := h.scopedWithRoleFor(ctx, params.Slug)
	if ent.IsNotFound(err) {
		v := siteapi.SiteContactsLookupNotFound(problem(http.StatusNotFound, "workspace not found"))
		return &v, nil
	}
	if err != nil {
		return nil, err
	}
	if !accounts.CanErase(role) {
		v := siteapi.SiteContactsLookupForbidden(problem(http.StatusForbidden, "insufficient role"))
		return &v, nil
	}

	held, err := erasure.Lookup(ctx, scoped, string(params.Email))
	if errors.Is(err, erasure.ErrInvalidEmail) {
		v := siteapi.SiteContactsLookupBadRequest(problem(http.StatusBadRequest, "invalid email address"))
		return &v, nil
	}
	if err != nil {
		return nil, err
	}
	return &siteapi.SiteContactLookup{
		ContactHeld:     held.ContactHeld,
		Events:          int32(held.Events),
		DeliveryRecords: int32(held.DeliveryRecords),
		Unsubscribes:    int32(held.Unsubscribes),
		Suppressions:    int32(held.Suppressions),
	}, nil
}
