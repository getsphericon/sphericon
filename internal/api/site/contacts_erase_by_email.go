package site

import (
	"context"
	"errors"
	"net/http"

	"github.com/getsphericon/sphericon/ent"
	siteapi "github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/accounts"
	"github.com/getsphericon/sphericon/internal/api/auth"
	"github.com/getsphericon/sphericon/internal/erasure"
)

// SiteContactsErase is Erasure by email address (ADR 0021, a Privacy request): the one
// erasure module, with the signed-in User as requester. Owner or admin only, like
// deleting a Contact. Not-found means neither a Contact nor delivery records hold the
// address.
func (h *Handlers) SiteContactsErase(ctx context.Context, params siteapi.SiteContactsEraseParams) (siteapi.SiteContactsEraseRes, error) {
	scoped, role, err := h.scopedWithRoleFor(ctx, params.Slug)
	if ent.IsNotFound(err) {
		v := siteapi.SiteContactsEraseNotFound(problem(http.StatusNotFound, "workspace not found"))
		return &v, nil
	}
	if err != nil {
		return nil, err
	}
	if !accounts.CanErase(role) {
		v := siteapi.SiteContactsEraseForbidden(problem(http.StatusForbidden, "insufficient role"))
		return &v, nil
	}

	email, err := erasure.NormalizeEmail(params.Email)
	if errors.Is(err, erasure.ErrInvalidEmail) {
		v := siteapi.SiteContactsEraseBadRequest(problem(http.StatusBadRequest, "invalid email address"))
		return &v, nil
	}
	if err != nil {
		return nil, err
	}

	err = h.erasure.Erase(ctx, scoped, erasure.ByEmail(email),
		erasure.Requester{Kind: erasure.RequesterUser, ID: auth.GetSiteAuth(ctx).UserID})
	if errors.Is(err, erasure.ErrNotFound) {
		v := siteapi.SiteContactsEraseNotFound(problem(http.StatusNotFound, "nothing found for the address"))
		return &v, nil
	}
	if err != nil {
		return nil, err
	}
	return &siteapi.SiteContactsEraseNoContent{}, nil
}
