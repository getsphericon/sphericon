package external

import (
	"context"
	"fmt"

	externalapi "github.com/getsphericon/sphericon/gen/external"
	"github.com/getsphericon/sphericon/internal/api/auth"
	"github.com/getsphericon/sphericon/internal/domain/identity/apitokens"
)

func (h *Handlers) AuthMeGet(ctx context.Context) (*externalapi.ApiTokenInfo, error) {
	// The security handler admitted the token, so the request carries it.
	token, err := apitokens.Get(ctx, auth.TokenScoped(ctx), auth.GetTokenAuth(ctx).TokenID)
	if err != nil {
		return nil, err
	}

	info := mapper.ApiTokenToInfo(token)
	return &info, nil
}

func (h *Handlers) AuthTokensList(ctx context.Context) (*externalapi.ApiTokenListResponse, error) {
	tokens, err := apitokens.List(ctx, auth.TokenScoped(ctx))
	if err != nil {
		return nil, err
	}

	items := mapper.ApiTokensToInfos(tokens)
	return &externalapi.ApiTokenListResponse{Items: items}, nil
}

func (h *Handlers) AuthTokensCreate(ctx context.Context, req *externalapi.CreateApiTokenInput) (*externalapi.CreateApiTokenResponse, error) {
	// A token can only mint scopes it holds itself (tokens:write is not a way to
	// gain send or any other power).
	minted, err := apitokens.MintWithin(ctx, auth.TokenScoped(ctx), auth.GetTokenAuth(ctx).Scopes, inputs.ApiTokenCreate(req))
	if err != nil {
		return nil, fmt.Errorf("create token: %w", err)
	}
	return mintedResponse(minted), nil
}

// AuthTokensBootstrap mints the Instance's first token, which no caller token can mint yet: the
// request presents the bootstrap token instead (apitokens.Module.Bootstrap).
func (h *Handlers) AuthTokensBootstrap(ctx context.Context, req *externalapi.CreateApiTokenInput, params externalapi.AuthTokensBootstrapParams) (*externalapi.CreateApiTokenResponse, error) {
	minted, err := h.tokens.Bootstrap(ctx, params.XBootstrapToken, inputs.ApiTokenCreate(req))
	if err != nil {
		return nil, err
	}
	return mintedResponse(minted), nil
}

func (h *Handlers) AuthTokensDelete(ctx context.Context, params externalapi.AuthTokensDeleteParams) error {
	id := int64(params.ID)

	err := apitokens.Revoke(ctx, auth.TokenScoped(ctx), id, h.now())
	if err != nil {
		return err
	}
	return nil
}

func mintedResponse(m apitokens.Minted) *externalapi.CreateApiTokenResponse {
	return &externalapi.CreateApiTokenResponse{
		Token:     m.Value,
		TokenInfo: mapper.ApiTokenToInfo(m.Token),
	}
}
