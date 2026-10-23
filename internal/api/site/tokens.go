package site

import (
	"context"
	"errors"
	"github.com/getsphericon/sphericon/internal/accounts"
	"github.com/samber/lo"
	"net/http"
	"time"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/apitoken"
	siteapi "github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/apitokens"
	"github.com/getsphericon/sphericon/internal/i18n"
)

// SiteTokensList returns the workspace's API tokens; a revoked one stays listed, marked by
// its revoked_at, so an owner sees a connector that was revoked (by hand or by reuse detection).
func (h *Handlers) SiteTokensList(ctx context.Context, params siteapi.SiteTokensListParams) (siteapi.SiteTokensListRes, error) {
	scoped, err := h.scopedFor(ctx, params.WorkspaceId)
	if ent.IsNotFound(err) {
		v := problem(http.StatusNotFound, "workspace not found")
		return &v, nil
	}
	if err != nil {
		return nil, err
	}

	tokens, err := scoped.ApiToken().Query().
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
	scoped, role, err := h.scopedWithRoleFor(ctx, params.WorkspaceId)
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

	in := apitokens.Input{Name: req.Name, Scopes: lo.Map(req.Scopes, func(s siteapi.ApiTokenScope, _ int) string { return string(s) })}
	if v, ok := req.ExpiresAt.Get(); ok {
		in.ExpiresAt = new(time.Time(v))
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
	case err != nil:
		return nil, err
	}

	return &siteapi.SiteCreateTokenResponse{
		Token:    minted.Value,
		Resource: mapper.TokenToResource(minted.Token),
	}, nil
}

// SiteTokensUpdate renames an active workspace API token.
func (h *Handlers) SiteTokensUpdate(ctx context.Context, req *siteapi.SiteUpdateTokenInput, params siteapi.SiteTokensUpdateParams) (siteapi.SiteTokensUpdateRes, error) {
	scoped, role, err := h.scopedWithRoleFor(ctx, params.WorkspaceId)
	if ent.IsNotFound(err) {
		v := siteapi.SiteTokensUpdateNotFound(problem(http.StatusNotFound, "workspace not found"))
		return &v, nil
	}
	if err != nil {
		return nil, err
	}
	if !accounts.CanManageTokens(role) {
		v := siteapi.SiteTokensUpdateForbidden(problem(http.StatusForbidden, "only owners and admins can manage API tokens"))
		return &v, nil
	}

	row, err := apitokens.Rename(ctx, scoped, int64(params.ID), req.Name)
	switch {
	case errors.Is(err, apitokens.ErrNameEmpty):
		v := siteapi.SiteTokensUpdateUnprocessableEntity(problemWithErrors(
			http.StatusUnprocessableEntity,
			i18n.T("errors.name_empty", nil),
			map[string][]string{"name": {i18n.T("errors.name_empty", nil)}},
		))
		return &v, nil
	case errors.Is(err, apitokens.ErrNotFound):
		v := siteapi.SiteTokensUpdateNotFound(problem(http.StatusNotFound, "token not found"))
		return &v, nil
	case err != nil:
		return nil, err
	}
	res := mapper.TokenToResource(row)
	return &res, nil
}

// SiteTokensDelete revokes (soft-deletes) a workspace API token.
func (h *Handlers) SiteTokensDelete(ctx context.Context, params siteapi.SiteTokensDeleteParams) (siteapi.SiteTokensDeleteRes, error) {
	scoped, role, err := h.scopedWithRoleFor(ctx, params.WorkspaceId)
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
