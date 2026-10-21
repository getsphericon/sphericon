package external

import (
	"context"
	"crypto/subtle"
	"errors"
	"net/http"
	"time"

	"github.com/getsphericon/sphericon/ent"
	externalapi "github.com/getsphericon/sphericon/gen/external"
	"github.com/getsphericon/sphericon/internal/api/auth"
	"github.com/getsphericon/sphericon/internal/apitokens"
	"github.com/samber/lo"
	"github.com/samber/oops"
)

func (h *Handlers) AuthMeGet(ctx context.Context) (externalapi.AuthMeGetRes, error) {
	a := auth.GetTokenAuth(ctx)
	if a == nil {
		res := externalapi.AuthMeGetUnauthorized(problem(http.StatusUnauthorized, "missing token"))
		return &res, nil
	}

	token, err := auth.TokenScoped(ctx).ApiToken().Get(ctx, a.TokenID)
	if ent.IsNotFound(err) {
		res := externalapi.AuthMeGetNotFound(problem(http.StatusNotFound, "token not found"))
		return &res, nil
	}
	if err != nil {
		return nil, err
	}

	info := mapper.ApiTokenToInfo(token)
	return &info, nil
}

func (h *Handlers) AuthTokensList(ctx context.Context) (externalapi.AuthTokensListRes, error) {
	if !auth.HasScope(auth.GetTokenAuth(ctx), externalapi.ApiTokenScopeTokensRead) {
		res := externalapi.AuthTokensListForbidden(problem(http.StatusForbidden, "insufficient scope"))
		return &res, nil
	}

	tokens, err := auth.TokenScoped(ctx).ApiToken().Query().All(ctx)
	if err != nil {
		return nil, err
	}

	items := make([]externalapi.ApiTokenInfo, len(tokens))
	for i, t := range tokens {
		items[i] = mapper.ApiTokenToInfo(t)
	}
	return &externalapi.ApiTokenListResponse{Items: items}, nil
}

func (h *Handlers) AuthTokensCreate(ctx context.Context, req *externalapi.CreateApiTokenInput) (externalapi.AuthTokensCreateRes, error) {
	if !auth.HasScope(auth.GetTokenAuth(ctx), externalapi.ApiTokenScopeTokensWrite) {
		res := externalapi.AuthTokensCreateForbidden(problem(http.StatusForbidden, "insufficient scope"))
		return &res, nil
	}

	// A token can only mint scopes it holds itself (tokens:write is not a way to
	// gain send or any other power).
	minted, err := apitokens.MintWithin(ctx, auth.TokenScoped(ctx), auth.GetTokenAuth(ctx).Scopes, mintInput(req))
	switch {
	case errors.Is(err, apitokens.ErrScopeEscalation):
		res := externalapi.AuthTokensCreateForbidden(problem(http.StatusForbidden, "cannot grant a scope this token does not have"))
		return &res, nil
	case errors.Is(err, apitokens.ErrNameEmpty), errors.Is(err, apitokens.ErrUnknownScope):
		res := externalapi.AuthTokensCreateBadRequest(problem(http.StatusBadRequest, err.Error()))
		return &res, nil
	case err != nil:
		return nil, oops.In("external-auth").Public("could not create token").Wrap(err)
	}
	return mintedResponse(minted), nil
}

func (h *Handlers) AuthTokensBootstrap(ctx context.Context, req *externalapi.CreateApiTokenInput, params externalapi.AuthTokensBootstrapParams) (externalapi.AuthTokensBootstrapRes, error) {
	if h.bootstrapToken == "" || subtle.ConstantTimeCompare([]byte(params.XBootstrapToken), []byte(h.bootstrapToken)) != 1 {
		res := externalapi.AuthTokensBootstrapUnauthorized(problem(http.StatusUnauthorized, "invalid bootstrap token"))
		return &res, nil
	}

	// Bootstrap has no caller token: the first token goes to the oldest workspace.
	s, err := h.accounts.BootstrapScope(ctx)
	if err != nil {
		return nil, oops.In("external-auth").Public("no workspace to bootstrap").Wrap(err)
	}
	minted, err := apitokens.Mint(ctx, s, mintInput(req))
	switch {
	case errors.Is(err, apitokens.ErrNameEmpty), errors.Is(err, apitokens.ErrUnknownScope):
		res := externalapi.AuthTokensBootstrapBadRequest(problem(http.StatusBadRequest, err.Error()))
		return &res, nil
	case err != nil:
		return nil, oops.In("external-auth").Public("could not create token").Wrap(err)
	}
	return mintedResponse(minted), nil
}

func (h *Handlers) AuthTokensDelete(ctx context.Context, params externalapi.AuthTokensDeleteParams) (externalapi.AuthTokensDeleteRes, error) {
	if !auth.HasScope(auth.GetTokenAuth(ctx), externalapi.ApiTokenScopeTokensWrite) {
		res := externalapi.AuthTokensDeleteForbidden(problem(http.StatusForbidden, "insufficient scope"))
		return &res, nil
	}

	id := int64(params.ID)

	err := apitokens.Revoke(ctx, auth.TokenScoped(ctx), id)
	if errors.Is(err, apitokens.ErrNotFound) {
		res := externalapi.AuthTokensDeleteNotFound(problem(http.StatusNotFound, "token not found"))
		return &res, nil
	}
	if err != nil {
		return nil, err
	}
	return &externalapi.AuthTokensDeleteNoContent{}, nil
}

func mintInput(req *externalapi.CreateApiTokenInput) apitokens.Input {
	in := apitokens.Input{
		Name:   req.Name,
		Scopes: lo.Map(req.Scopes, func(sc externalapi.ApiTokenScope, _ int) string { return string(sc) }),
	}
	if v, ok := req.ExpiresAt.Get(); ok {
		in.ExpiresAt = new(time.Time(v))
	}
	return in
}

func mintedResponse(m apitokens.Minted) *externalapi.CreateApiTokenResponse {
	return &externalapi.CreateApiTokenResponse{
		Token:     m.Value,
		TokenInfo: mapper.ApiTokenToInfo(m.Token),
	}
}
