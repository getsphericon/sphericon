package site

import (
	"context"
	"time"

	"github.com/samber/lo"

	siteapi "github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/apitokens"
)

// SiteTokensList returns the workspace's API tokens (Connections are listed apart, ADR 0061); a
// revoked one stays listed, marked by its revoked_at.
func (h *Handlers) SiteTokensList(ctx context.Context, _ siteapi.SiteTokensListParams) (siteapi.SiteTokensListRes, error) {
	scoped := workspaceScope(ctx)

	tokens, err := apitokens.List(ctx, scoped)
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
// only its SHA-256 hash and public prefix are stored.
func (h *Handlers) SiteTokensCreate(ctx context.Context, req *siteapi.SiteCreateTokenInput, _ siteapi.SiteTokensCreateParams) (siteapi.SiteTokensCreateRes, error) {
	scoped := workspaceScope(ctx)

	in := apitokens.Input{Name: req.Name, Scopes: lo.Map(req.Scopes, func(s siteapi.ApiTokenScope, _ int) string { return string(s) })}
	if v, ok := req.ExpiresAt.Get(); ok {
		in.ExpiresAt = new(time.Time(v))
	}
	// Owners and admins may grant any scope of the vocabulary.
	minted, err := apitokens.Mint(ctx, scoped, in)
	if err != nil {
		return nil, err
	}

	return &siteapi.SiteCreateTokenResponse{
		Token:    minted.Value,
		Resource: mapper.TokenToResource(minted.Token),
	}, nil
}

// SiteTokensUpdate renames an active workspace API token.
func (h *Handlers) SiteTokensUpdate(ctx context.Context, req *siteapi.SiteUpdateTokenInput, params siteapi.SiteTokensUpdateParams) (siteapi.SiteTokensUpdateRes, error) {
	scoped := workspaceScope(ctx)

	row, err := apitokens.Rename(ctx, scoped, int64(params.ID), req.Name)
	if err != nil {
		return nil, err
	}
	res := mapper.TokenToResource(row)
	return &res, nil
}

// SiteTokensDelete revokes (soft-deletes) a workspace API token.
func (h *Handlers) SiteTokensDelete(ctx context.Context, params siteapi.SiteTokensDeleteParams) (siteapi.SiteTokensDeleteRes, error) {
	scoped := workspaceScope(ctx)

	id := int64(params.ID)

	// An unknown, foreign or already-revoked id is a 404.
	err := apitokens.Revoke(ctx, scoped, id)
	if err != nil {
		return nil, err
	}
	return &siteapi.SiteTokensDeleteNoContent{}, nil
}
