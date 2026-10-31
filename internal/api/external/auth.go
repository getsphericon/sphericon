package external

import (
	"context"
	"fmt"

	externalapi "github.com/getsphericon/sphericon/gen/external"
	"github.com/getsphericon/sphericon/internal/api/auth"
	"github.com/getsphericon/sphericon/internal/apitokens"
)

func (h *Handlers) AuthMeGet(ctx context.Context) (externalapi.AuthMeGetRes, error) {
	// The security handler admitted the token, so the request carries it.
	token, err := apitokens.Get(ctx, auth.TokenScoped(ctx), auth.GetTokenAuth(ctx).TokenID)
	if err != nil {
		return nil, err
	}

	info := mapper.ApiTokenToInfo(token)
	return &info, nil
}

func (h *Handlers) AuthTokensList(ctx context.Context) (externalapi.AuthTokensListRes, error) {
	tokens, err := apitokens.List(ctx, auth.TokenScoped(ctx))
	if err != nil {
		return nil, err
	}

	items := mapper.ApiTokensToInfos(tokens)
	return &externalapi.ApiTokenListResponse{Items: items}, nil
}

func (h *Handlers) AuthTokensCreate(ctx context.Context, req *externalapi.CreateApiTokenInput) (externalapi.AuthTokensCreateRes, error) {
	// A token can only mint scopes it holds itself (tokens:write is not a way to
	// gain send or any other power).
	minted, err := apitokens.MintWithin(ctx, auth.TokenScoped(ctx), auth.GetTokenAuth(ctx).Scopes, inputs.ApiTokenCreate(req))
	if err != nil {
		return nil, fmt.Errorf("create token: %w", err)
	}
	return mintedResponse(minted), nil
}

func (h *Handlers) AuthTokensBootstrap(ctx context.Context, req *externalapi.CreateApiTokenInput, params externalapi.AuthTokensBootstrapParams) (externalapi.AuthTokensBootstrapRes, error) {
	if err := apitokens.CheckBootstrap(h.bootstrapToken, params.XBootstrapToken); err != nil {
		return nil, err
	}

	// Bootstrap has no caller token: the first token goes to the oldest workspace.
	s, err := h.accounts.BootstrapScope(ctx)
	if err != nil {
		return nil, fmt.Errorf("bootstrap scope: %w", err)
	}
	minted, err := apitokens.Mint(ctx, s, inputs.ApiTokenCreate(req))
	if err != nil {
		return nil, fmt.Errorf("create token: %w", err)
	}
	return mintedResponse(minted), nil
}

func (h *Handlers) AuthTokensDelete(ctx context.Context, params externalapi.AuthTokensDeleteParams) (externalapi.AuthTokensDeleteRes, error) {
	id := int64(params.ID)

	err := apitokens.Revoke(ctx, auth.TokenScoped(ctx), id)
	if err != nil {
		return nil, err
	}
	return &externalapi.AuthTokensDeleteNoContent{}, nil
}

func mintedResponse(m apitokens.Minted) *externalapi.CreateApiTokenResponse {
	return &externalapi.CreateApiTokenResponse{
		Token:     m.Value,
		TokenInfo: mapper.ApiTokenToInfo(m.Token),
	}
}
