package site

import (
	"context"

	siteapi "github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/brandkit"
)

// SiteBrandKitGet returns the Workspace's Brand kit; every member may read it.
func (h *Handlers) SiteBrandKitGet(ctx context.Context, _ siteapi.SiteBrandKitGetParams) (siteapi.SiteBrandKitGetRes, error) {
	s := workspaceScope(ctx)
	kit, err := h.brandKit.Get(ctx, s)
	if err != nil {
		return nil, err
	}
	res := mapper.BrandKitToResource(kit)
	return &res, nil
}

// SiteBrandKitSave saves the Brand kit: the first save also makes the starter Templates.
// Permission brand_kit:manage.
func (h *Handlers) SiteBrandKitSave(ctx context.Context, req *siteapi.SiteSaveBrandKitInput, _ siteapi.SiteBrandKitSaveParams) (siteapi.SiteBrandKitSaveRes, error) {
	s := workspaceScope(ctx)

	kit, err := h.brandKit.Save(ctx, s, brandkit.Input{
		AccentColor:     req.AccentColor,
		TextColor:       req.TextColor,
		BackgroundColor: req.BackgroundColor,
		HeadingFont:     req.HeadingFont,
		BodyFont:        req.BodyFont,
		LogoURL:         req.LogoUrl,
		ButtonTextColor: req.ButtonTextColor,
		ButtonRadius:    int(req.ButtonRadius),
		FooterText:      req.FooterText,
	})
	if err != nil {
		return nil, err
	}
	res := mapper.BrandKitToResource(kit)
	return &res, nil
}

// SiteBrandKitRecreateStarterTemplates makes the starter Templates again from the saved Brand
// kit. Permission brand_kit:manage.
func (h *Handlers) SiteBrandKitRecreateStarterTemplates(ctx context.Context, _ siteapi.SiteBrandKitRecreateStarterTemplatesParams) (siteapi.SiteBrandKitRecreateStarterTemplatesRes, error) {
	s := workspaceScope(ctx)

	made, err := h.brandKit.RecreateStarterTemplates(ctx, s)
	if err != nil {
		return nil, err
	}
	res := siteapi.SiteBrandKitRecreateStarterTemplatesCreatedApplicationJSON(
		mapper.EmailTemplatesToResources(made))
	return &res, nil
}
