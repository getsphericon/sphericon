package site

import (
	"context"

	siteapi "github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/domain/identity/apitokens"
)

// SitePersonalTokensList returns the Workspace's active Personal tokens the caller may see: their
// own, or every member's with members:manage (the Policy of apitokens.PersonalTokens).
func (h *Handlers) SitePersonalTokensList(ctx context.Context, _ siteapi.SitePersonalTokensListParams) ([]siteapi.SitePersonalTokenResource, error) {
	tokens, err := apitokens.PersonalTokens(ctx, workspaceScope(ctx), callerMembership(ctx))
	if err != nil {
		return nil, err
	}
	items := mapper.PersonalTokensToResources(tokens)
	return items, nil
}

// SitePersonalTokensCreate mints a Personal token of the caller's own. The full secret is returned
// once; only its SHA-256 hash and public prefix are stored.
func (h *Handlers) SitePersonalTokensCreate(ctx context.Context, req *siteapi.SiteCreatePersonalTokenInput, _ siteapi.SitePersonalTokensCreateParams) (*siteapi.SiteCreatePersonalTokenResponse, error) {
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
func (h *Handlers) SitePersonalTokensDelete(ctx context.Context, params siteapi.SitePersonalTokensDeleteParams) error {
	return apitokens.RevokePersonal(ctx, workspaceScope(ctx), callerMembership(ctx), int64(params.ID))
}

// SitePersonalTokensGrantableScopes returns the scopes the caller's Role may give a Personal token.
func (h *Handlers) SitePersonalTokensGrantableScopes(ctx context.Context, _ siteapi.SitePersonalTokensGrantableScopesParams) ([]siteapi.PersonalScope, error) {
	return mapper.PersonalScopesToResources(apitokens.GrantableScopes(callerMembership(ctx).Role)), nil
}
