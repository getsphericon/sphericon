package site

import (
	"context"
	"net/http"

	"github.com/getsphericon/sphericon/ent"
	siteapi "github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/accounts"
	"github.com/getsphericon/sphericon/internal/setupchecklist"
)

// SiteSetupChecklistGet maps the Workspace's Setup checklist onto the DTO.
func (h *Handlers) SiteSetupChecklistGet(ctx context.Context, params siteapi.SiteSetupChecklistGetParams) (siteapi.SiteSetupChecklistGetRes, error) {
	s, err := h.scopedFor(ctx, params.Slug)
	if ent.IsNotFound(err) {
		v := problem(http.StatusNotFound, "workspace not found")
		return &v, nil
	}
	if err != nil {
		return nil, err
	}
	c, err := setupchecklist.Read(ctx, s)
	if err != nil {
		return nil, err
	}
	return setupProgress(c), nil
}

// SiteSetupChecklistDismiss dismisses the Setup checklist for the whole Workspace; owner
// and admin only, idempotent. It answers the checklist as it now reads.
func (h *Handlers) SiteSetupChecklistDismiss(ctx context.Context, params siteapi.SiteSetupChecklistDismissParams) (siteapi.SiteSetupChecklistDismissRes, error) {
	s, role, err := h.scopedWithRoleFor(ctx, params.Slug)
	if ent.IsNotFound(err) {
		v := siteapi.SiteSetupChecklistDismissNotFound(problem(http.StatusNotFound, "workspace not found"))
		return &v, nil
	}
	if err != nil {
		return nil, err
	}
	if !accounts.CanManageMembers(role) {
		v := siteapi.SiteSetupChecklistDismissForbidden(problem(http.StatusForbidden, "only owners and admins can dismiss the setup checklist"))
		return &v, nil
	}
	if err := h.accounts.DismissSetupChecklist(ctx, s, h.actor(ctx), h.now()); err != nil {
		return nil, err
	}
	c, err := setupchecklist.Read(ctx, s)
	if err != nil {
		return nil, err
	}
	return setupProgress(c), nil
}

func setupProgress(c setupchecklist.Checklist) *siteapi.SiteSetupProgress {
	return &siteapi.SiteSetupProgress{
		Steps: siteapi.SiteSetupSteps{
			Integration:   siteapi.SiteSetupStepStatus(c.Steps.Integration),
			SendingDomain: siteapi.SiteSetupStepStatus(c.Steps.SendingDomain),
			Contact:       siteapi.SiteSetupStepStatus(c.Steps.Contact),
			SentEmail:     siteapi.SiteSetupStepStatus(c.Steps.SentEmail),
		},
		Complete:  c.Complete(),
		Dismissed: c.Dismissed,
	}
}
