package site

import (
	"context"

	siteapi "github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/api/auth"
	"github.com/getsphericon/sphericon/internal/usecase/contacterasure"
)

// SiteContactsErase is Erasure by email address (ADR 0021, a Privacy request): the one
// Contact erasure use case, with the signed-in User as requester. Owner or admin only, like
// deleting a Contact. Not found means neither a Contact nor delivery records hold the
// address; an invalid address is refused (contacterasure.ErrInvalidEmail) before anything is
// erased.
func (h *Handlers) SiteContactsErase(ctx context.Context, params siteapi.SiteContactsEraseParams) error {
	err := h.erasure.Erase(ctx, workspaceScope(ctx), contacterasure.Identifier{Email: &params.Email},
		contacterasure.Requester{Kind: contacterasure.RequesterUser, ID: auth.GetSiteAuth(ctx).UserID})
	if err != nil {
		return err
	}
	return nil
}
