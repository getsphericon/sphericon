package site

import (
	"context"

	siteapi "github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/kernel/narrow"
	"github.com/getsphericon/sphericon/internal/usecase/contacterasure"
)

// SiteContactsLookup answers a Privacy request's first question (ADR 0021): what the
// Workspace holds for an email address, as counts. It sits beside Erasure, so owner
// or admin only; an address nothing holds answers with zeros.
func (h *Handlers) SiteContactsLookup(ctx context.Context, params siteapi.SiteContactsLookupParams) (*siteapi.SiteContactHoldings, error) {
	// Lookup refuses an invalid address (contacterasure.ErrInvalidEmail).
	held, err := contacterasure.Lookup(ctx, workspaceScope(ctx), params.Email)
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
