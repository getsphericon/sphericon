package external

import (
	"context"
	"net/http"

	externalapi "github.com/getsphericon/sphericon/gen/external"
	"github.com/getsphericon/sphericon/internal/api/auth"
)

// BrandKitGet returns the Workspace's Brand kit for an agent to write a message in (ADR 0040). It
// needs only the right to read Templates; the Brand kit is never written over /api.
func (h *Handlers) BrandKitGet(ctx context.Context) (externalapi.BrandKitGetRes, error) {
	if !auth.HasScope(auth.GetTokenAuth(ctx), externalapi.ApiTokenScopeTemplatesRead) {
		res := externalapi.ProblemDetails(problem(http.StatusUnauthorized, "insufficient scope"))
		return &res, nil
	}
	kit, err := h.brandKit.Get(ctx, auth.TokenScoped(ctx))
	if err != nil {
		return nil, err
	}
	res := mapper.BrandKitToResource(kit)
	return &res, nil
}
