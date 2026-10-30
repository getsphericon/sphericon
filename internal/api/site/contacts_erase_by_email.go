package site

import (
	"context"

	siteapi "github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/api/auth"
	"github.com/getsphericon/sphericon/internal/erasure"
)

// SiteContactsErase is Erasure by email address (ADR 0021, a Privacy request): the one
// erasure module, with the signed-in User as requester. Owner or admin only, like
// deleting a Contact. Not found means neither a Contact nor delivery records hold the
// address.
func (h *Handlers) SiteContactsErase(ctx context.Context, params siteapi.SiteContactsEraseParams) (siteapi.SiteContactsEraseRes, error) {
	// An invalid address is refused before anything is erased.
	if _, err := erasure.NormalizeEmail(params.Email); err != nil {
		return nil, err
	}
	err := h.erasure.Erase(ctx, workspaceScope(ctx), erasure.ByEmail(params.Email),
		erasure.Requester{Kind: erasure.RequesterUser, ID: auth.GetSiteAuth(ctx).UserID})
	if err != nil {
		return nil, err
	}
	return &siteapi.SiteContactsEraseNoContent{}, nil
}
