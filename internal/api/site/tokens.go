package site

import (
	"context"
	"errors"
	"github.com/getsphericon/sphericon/internal/accounts"
	"net/http"
	"time"

	"github.com/samber/lo"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/apitoken"
	siteapi "github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/apitokens"
	"github.com/getsphericon/sphericon/internal/i18n"
)

// SiteTokensList returns the workspace's active (non-revoked) API tokens.
func (h *Handlers) SiteTokensList(ctx context.Context, params siteapi.SiteTokensListParams) (siteapi.SiteTokensListRes, error) {
	scoped, err := h.scopedFor(ctx, params.Slug)
	if ent.IsNotFound(err) {
		v := problem(http.StatusNotFound, "workspace not found")
		return &v, nil
	}
	if err != nil {
		return nil, err
	}

	tokens, err := scoped.ApiToken().Query().
		Where(apitoken.RevokedAtIsNil()).
		Order(ent.Asc(apitoken.FieldID)).
		All(ctx)
	if err != nil {
		return nil, err
	}

	items := make(siteapi.SiteTokensListOKApplicationJSON, len(tokens))
	for i, t := range tokens {
		items[i] = mapper.TokenToResource(t)
	}
	return &items, nil
}

// SiteTokensCreate mints a workspace API token. The full secret is returned once;
// only its bcrypt hash and public prefix are stored.
func (h *Handlers) SiteTokensCreate(ctx context.Context, req *siteapi.SiteCreateTokenInput, params siteapi.SiteTokensCreateParams) (siteapi.SiteTokensCreateRes, error) {
	scoped, role, err := h.scopedWithRoleFor(ctx, params.Slug)
	if ent.IsNotFound(err) {
		v := siteapi.SiteTokensCreateNotFound(problem(http.StatusNotFound, "workspace not found"))
		return &v, nil
	}
	if err != nil {
		return nil, err
	}
	if !accounts.CanManageTokens(role) {
		v := siteapi.SiteTokensCreateForbidden(problem(http.StatusForbidden, "only owners and admins can manage API tokens"))
		return &v, nil
	}

	in := apitokens.Input{Name: req.Name, Scopes: req.Scopes}
	if v, ok := req.ExpiresAt.Get(); ok {
		in.ExpiresAt = lo.ToPtr(time.Time(v))
	}
	// Owners and admins may grant any scope of the vocabulary.
	minted, err := apitokens.Mint(ctx, scoped, in)
	switch {
	case errors.Is(err, apitokens.ErrNameEmpty):
		v := siteapi.SiteTokensCreateUnprocessableEntity(problemWithErrors(
			http.StatusUnprocessableEntity,
			i18n.T("errors.name_empty", nil),
			map[string][]string{"name": {i18n.T("errors.name_empty", nil)}},
		))
		return &v, nil
	case errors.Is(err, apitokens.ErrUnknownScope):
		v := siteapi.SiteTokensCreateUnprocessableEntity(problemWithErrors(
			http.StatusUnprocessableEntity,
			"unknown scope",
			map[string][]string{"scopes": {"unknown scope"}},
		))
		return &v, nil
	case err != nil:
		return nil, err
	}

	return &siteapi.SiteCreateTokenResponse{
		Token:    minted.Value,
		Resource: mapper.TokenToResource(minted.Token),
	}, nil
}

// SiteTokensDelete revokes (soft-deletes) a workspace API token.
func (h *Handlers) SiteTokensDelete(ctx context.Context, params siteapi.SiteTokensDeleteParams) (siteapi.SiteTokensDeleteRes, error) {
	scoped, role, err := h.scopedWithRoleFor(ctx, params.Slug)
	if ent.IsNotFound(err) {
		v := siteapi.SiteTokensDeleteNotFound(problem(http.StatusNotFound, "workspace not found"))
		return &v, nil
	}
	if err != nil {
		return nil, err
	}
	if !accounts.CanManageTokens(role) {
		v := siteapi.SiteTokensDeleteForbidden(problem(http.StatusForbidden, "only owners and admins can manage API tokens"))
		return &v, nil
	}

	id := int64(params.ID)

	// An unknown, foreign or already-revoked id is a 404.
	err = apitokens.Revoke(ctx, scoped, id)
	if errors.Is(err, apitokens.ErrNotFound) {
		v := siteapi.SiteTokensDeleteNotFound(problem(http.StatusNotFound, "token not found"))
		return &v, nil
	}
	if err != nil {
		return nil, err
	}
	return &siteapi.SiteTokensDeleteNoContent{}, nil
}
