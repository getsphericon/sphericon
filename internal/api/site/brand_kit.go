package site

import (
	"context"

	"github.com/samber/lo"

	"github.com/getsphericon/sphericon/ent"
	siteapi "github.com/getsphericon/sphericon/gen/site"
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

	kit, err := h.brandKit.Save(ctx, s, inputs.BrandKitSave(req))
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
		lo.Map(made, func(t *ent.EmailTemplate, _ int) siteapi.SiteEmailTemplateResource {
			return mapper.EmailTemplateToResource(t)
		}))
	return &res, nil
}
