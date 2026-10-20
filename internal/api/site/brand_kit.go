package site

import (
	"context"
	"errors"
	"net/http"

	"github.com/samber/lo"

	"github.com/getsphericon/sphericon/ent"
	siteapi "github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/accounts"
	"github.com/getsphericon/sphericon/internal/brandkit"
	"github.com/getsphericon/sphericon/internal/emailrender"
	"github.com/getsphericon/sphericon/internal/i18n"
)

const errBrandKitForbidden = "only owners can change the Brand kit"

// brandKitResource maps the module's Kit to the API resource, by hand: a Workspace with no saved
// Brand kit has no row to convert.
func brandKitResource(k brandkit.Kit) siteapi.SiteBrandKitResource {
	res := siteapi.SiteBrandKitResource{
		Saved:           k.Saved,
		AccentColor:     k.AccentColor,
		TextColor:       k.TextColor,
		BackgroundColor: k.BackgroundColor,
		HeadingFont:     k.HeadingFont,
		BodyFont:        k.BodyFont,
		LogoUrl:         k.LogoURL,
		ButtonTextColor: k.ButtonTextColor,
		ButtonRadius:    int32(k.ButtonRadius),
		FooterText:      k.FooterText,
	}
	if k.Saved {
		res.UpdatedAt = siteapi.NewOptNilTimestamp(siteapi.Timestamp(k.UpdatedAt))
	}
	return res
}

// SiteBrandKitGet returns the Workspace's Brand kit; every member may read it.
func (h *Handlers) SiteBrandKitGet(ctx context.Context, params siteapi.SiteBrandKitGetParams) (siteapi.SiteBrandKitGetRes, error) {
	s, err := h.scopedFor(ctx, params.WorkspaceId)
	if ent.IsNotFound(err) {
		v := problem(http.StatusNotFound, "workspace not found")
		return &v, nil
	}
	if err != nil {
		return nil, err
	}
	kit, err := h.brandKit.Get(ctx, s)
	if err != nil {
		return nil, err
	}
	res := brandKitResource(kit)
	return &res, nil
}

// SiteBrandKitSave saves the Brand kit: the first save also makes the starter Templates. Owners
// only.
func (h *Handlers) SiteBrandKitSave(ctx context.Context, req *siteapi.SiteSaveBrandKitInput, params siteapi.SiteBrandKitSaveParams) (siteapi.SiteBrandKitSaveRes, error) {
	s, role, err := h.scopedWithRoleFor(ctx, params.WorkspaceId)
	if ent.IsNotFound(err) {
		v := siteapi.SiteBrandKitSaveNotFound(problem(http.StatusNotFound, "workspace not found"))
		return &v, nil
	}
	if err != nil {
		return nil, err
	}
	if !accounts.CanManageBrandKit(role) {
		v := siteapi.SiteBrandKitSaveForbidden(problem(http.StatusForbidden, errBrandKitForbidden))
		return &v, nil
	}

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
	var verr *brandkit.ValidationError
	switch {
	case errors.As(err, &verr):
		v := siteapi.SiteBrandKitSaveUnprocessableEntity(problemWithErrors(
			http.StatusUnprocessableEntity, verr.Message, map[string][]string{verr.Field: {verr.Message}},
		))
		return &v, nil
	case errors.Is(err, emailrender.ErrInvalidContent):
		v := siteapi.SiteBrandKitSaveUnprocessableEntity(problem(http.StatusUnprocessableEntity, err.Error()))
		return &v, nil
	case err != nil:
		return nil, err
	}
	res := brandKitResource(kit)
	return &res, nil
}

// SiteBrandKitRecreateStarterTemplates makes the starter Templates again from the saved Brand
// kit. Owners only.
func (h *Handlers) SiteBrandKitRecreateStarterTemplates(ctx context.Context, params siteapi.SiteBrandKitRecreateStarterTemplatesParams) (siteapi.SiteBrandKitRecreateStarterTemplatesRes, error) {
	s, role, err := h.scopedWithRoleFor(ctx, params.WorkspaceId)
	if ent.IsNotFound(err) {
		v := siteapi.SiteBrandKitRecreateStarterTemplatesNotFound(problem(http.StatusNotFound, "workspace not found"))
		return &v, nil
	}
	if err != nil {
		return nil, err
	}
	if !accounts.CanManageBrandKit(role) {
		v := siteapi.SiteBrandKitRecreateStarterTemplatesForbidden(problem(http.StatusForbidden, errBrandKitForbidden))
		return &v, nil
	}

	made, err := h.brandKit.RecreateStarterTemplates(ctx, s)
	switch {
	case errors.Is(err, brandkit.ErrNotSaved):
		v := siteapi.SiteBrandKitRecreateStarterTemplatesUnprocessableEntity(
			problem(http.StatusUnprocessableEntity, i18n.T("errors.brandkit_not_saved", nil)))
		return &v, nil
	case errors.Is(err, emailrender.ErrInvalidContent):
		v := siteapi.SiteBrandKitRecreateStarterTemplatesUnprocessableEntity(problem(http.StatusUnprocessableEntity, err.Error()))
		return &v, nil
	case err != nil:
		return nil, err
	}
	res := siteapi.SiteBrandKitRecreateStarterTemplatesCreatedApplicationJSON(
		lo.Map(made, func(t *ent.EmailTemplate, _ int) siteapi.SiteEmailTemplateResource {
			return mapper.EmailTemplateToResource(t)
		}))
	return &res, nil
}
