package site

import (
	"context"

	siteapi "github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/apitokens"
)

// SitePersonalTokensList returns the Workspace's active Personal tokens the caller may see: their
// own, or every member's with members:manage (the Policy of apitokens.PersonalTokens).
func (h *Handlers) SitePersonalTokensList(ctx context.Context, _ siteapi.SitePersonalTokensListParams) (siteapi.SitePersonalTokensListRes, error) {
	tokens, err := apitokens.PersonalTokens(ctx, workspaceScope(ctx), callerMembership(ctx))
	if err != nil {
		return nil, err
	}
	items := make(siteapi.SitePersonalTokensListOKApplicationJSON, len(tokens))
	for i, t := range tokens {
		items[i] = mapper.PersonalTokenToResource(t)
	}
	return &items, nil
}

// SitePersonalTokensCreate mints a Personal token of the caller's own. The full secret is returned
// once; only its SHA-256 hash and public prefix are stored.
func (h *Handlers) SitePersonalTokensCreate(ctx context.Context, req *siteapi.SiteCreatePersonalTokenInput, _ siteapi.SitePersonalTokensCreateParams) (siteapi.SitePersonalTokensCreateRes, error) {
	minted, err := apitokens.MintPersonal(ctx, workspaceScope(ctx), callerMembership(ctx), inputs.PersonalTokenCreate(req))
	if err != nil {
		return nil, err
	}
	return &siteapi.SiteCreatePersonalTokenResponse{
		Token:    minted.Value,
		Resource: mapper.PersonalTokenToResource(minted.Token),
	}, nil
}

// SitePersonalTokensDelete revokes a Personal token: the caller's own, or anyone's with
// members:manage.
func (h *Handlers) SitePersonalTokensDelete(ctx context.Context, params siteapi.SitePersonalTokensDeleteParams) (siteapi.SitePersonalTokensDeleteRes, error) {
	if err := apitokens.RevokePersonal(ctx, workspaceScope(ctx), callerMembership(ctx), int64(params.ID)); err != nil {
		return nil, err
	}
	return &siteapi.SitePersonalTokensDeleteNoContent{}, nil
}

// SitePersonalTokensGrantableScopes returns the scopes the caller's Role may give a Personal token.
func (h *Handlers) SitePersonalTokensGrantableScopes(ctx context.Context, _ siteapi.SitePersonalTokensGrantableScopesParams) (siteapi.SitePersonalTokensGrantableScopesRes, error) {
	items := siteapi.SitePersonalTokensGrantableScopesOKApplicationJSON(apitokens.GrantableScopes(callerMembership(ctx).Role))
	return &items, nil
}
