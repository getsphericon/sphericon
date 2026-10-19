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
	if !auth.HasScope(auth.GetTokenAuth(ctx), "templates:read") {
		res := externalapi.ProblemDetails(problem(http.StatusUnauthorized, "insufficient scope"))
		return &res, nil
	}
	kit, err := h.brandKit.Get(ctx, auth.TokenScoped(ctx))
	if err != nil {
		return nil, err
	}
	res := externalapi.BrandKitResource{
		Saved:           kit.Saved,
		AccentColor:     kit.AccentColor,
		TextColor:       kit.TextColor,
		BackgroundColor: kit.BackgroundColor,
		HeadingFont:     kit.HeadingFont,
		BodyFont:        kit.BodyFont,
		LogoUrl:         kit.LogoURL,
		ButtonTextColor: kit.ButtonTextColor,
		ButtonRadius:    int32(kit.ButtonRadius),
		FooterText:      kit.FooterText,
	}
	if kit.Saved {
		res.UpdatedAt = externalapi.NewOptNilTimestamp(externalapi.Timestamp(kit.UpdatedAt))
	}
	return &res, nil
}
