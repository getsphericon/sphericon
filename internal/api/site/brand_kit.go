package site

import (
	"context"

	siteapi "github.com/getsphericon/sphericon/gen/site"
)

// SiteBrandKitGet returns the Workspace's Brand kit; every member may read it.
func (h *Handlers) SiteBrandKitGet(ctx context.Context, _ siteapi.SiteBrandKitGetParams) (*siteapi.SiteBrandKitResource, error) {
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
func (h *Handlers) SiteBrandKitSave(ctx context.Context, req *siteapi.SiteSaveBrandKitInput, _ siteapi.SiteBrandKitSaveParams) (*siteapi.SiteBrandKitResource, error) {
	s := workspaceScope(ctx)

	kit, err := h.brandKit.Save(ctx, s, inputs.BrandKitSave(req))
	if err != nil {
		return nil, err
	}
	res := mapper.BrandKitToResource(kit)
	return &res, nil
}

// SiteBrandKitRecreateStarterTemplates makes the starter Templates again from the saved Brand
// kit. Permission brand_kit:manage.
func (h *Handlers) SiteBrandKitRecreateStarterTemplates(ctx context.Context, _ siteapi.SiteBrandKitRecreateStarterTemplatesParams) ([]siteapi.SiteEmailTemplateResource, error) {
	s := workspaceScope(ctx)

	made, err := h.brandKit.RecreateStarterTemplates(ctx, s)
	if err != nil {
		return nil, err
	}
	return mapper.EmailTemplatesToResources(made), nil
}
