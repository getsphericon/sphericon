package site

import (
	"context"
	"errors"
	"net/http"

	siteapi "github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/api/auth"
	"github.com/getsphericon/sphericon/internal/erasure"
)

// SiteContactsErase is Erasure by email address (ADR 0021, a Privacy request): the one
// erasure module, with the signed-in User as requester. Owner or admin only, like
// deleting a Contact. Not-found means neither a Contact nor delivery records hold the
// address.
func (h *Handlers) SiteContactsErase(ctx context.Context, params siteapi.SiteContactsEraseParams) (siteapi.SiteContactsEraseRes, error) {
	scoped, refusal, err := h.privacyScope(ctx, params.WorkspaceId, params.Email)
	if err != nil {
		return nil, err
	}
	if refusal != nil {
		switch refusal.Status.Value {
		case http.StatusForbidden:
			v := siteapi.SiteContactsEraseForbidden(*refusal)
			return &v, nil
		default:
			v := siteapi.SiteContactsEraseBadRequest(*refusal)
			return &v, nil
		}
	}

	err = h.erasure.Erase(ctx, scoped, erasure.ByEmail(params.Email),
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
