package site

import (
	"context"

	siteapi "github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/domain/orchestration/automations"
	"github.com/getsphericon/sphericon/internal/kernel/pagination"
)

func (h *Handlers) SiteAutomationsList(ctx context.Context, params siteapi.SiteAutomationsListParams) (*siteapi.SiteAutomationsListOK, error) {
	ws := workspaceScope(ctx)

	page, err := h.automations.ListWithRevenue(ctx, ws, pagination.ParamsOf(params.Page, params.PageSize), inputs.AutomationsListFilter(params))
	if err != nil {
		return nil, err
	}
	return mapper.AutomationsListPage(page)
}

func (h *Handlers) SiteAutomationsCreate(ctx context.Context, req *siteapi.CreateAutomationInput, _ siteapi.SiteAutomationsCreateParams) (*siteapi.SiteAutomationResource, error) {
	ws := workspaceScope(ctx)

	in, err := inputs.AutomationCreate(req)
	if err != nil {
		return nil, err
	}
	l, err := h.automationViews.Create(ctx, ws, in)
	if err != nil {
		return nil, err
	}
	return savedAutomation(l)
}

func (h *Handlers) SiteAutomationsGet(ctx context.Context, params siteapi.SiteAutomationsGetParams) (*siteapi.SiteAutomationResource, error) {
	ws := workspaceScope(ctx)

	l, err := h.automationViews.Get(ctx, ws, int64(params.ID))
	if err != nil {
		return nil, err
	}
	res, err := mapper.AutomationWithRevenueToResource(l.WithRevenue)
	if err != nil {
		return nil, err
	}
	res.Warnings = mapper.AutomationStepsWarningsToResource(l.Warnings)
	return &res, nil
}

func (h *Handlers) SiteAutomationsUpdate(ctx context.Context, req *siteapi.UpdateAutomationInput, params siteapi.SiteAutomationsUpdateParams) (*siteapi.SiteAutomationResource, error) {
	ws := workspaceScope(ctx)

	id := int64(params.ID)
	in, err := inputs.AutomationUpdate(req)
	if err != nil {
		return nil, err
	}
	l, err := h.automationViews.Update(ctx, ws, id, in)
	if err != nil {
		return nil, err
	}
	return savedAutomation(l)
}

// savedAutomation maps a saved Automation, which a change answers without its revenue, and attaches
// the lint warnings of its email steps (ADR 0040).
func savedAutomation(l automations.Linted) (*siteapi.SiteAutomationResource, error) {
	res, err := mapper.AutomationToResource(l.Automation)
	if err != nil {
		return nil, err
	}
	res.Warnings = mapper.AutomationStepsWarningsToResource(l.Warnings)
	return &res, nil
}

// SiteAutomationsArchive archives an automation (ADR 0100): it leaves the list and enrolls
// nobody, and keeps its enrollments. Live use (active enrollments, an active rule) refuses it with
// 409.
func (h *Handlers) SiteAutomationsArchive(ctx context.Context, params siteapi.SiteAutomationsArchiveParams) (*siteapi.SiteAutomationResource, error) {
	ws := workspaceScope(ctx)

	id := int64(params.ID)
	a, err := h.automations.Archive(ctx, ws, id)
	if err != nil {
		return nil, err
	}
	res, err := mapper.AutomationToResource(a)
	if err != nil {
		return nil, err
	}
	return &res, nil
}

// SiteAutomationsUnarchive brings an archived automation back to the list, inactive.
func (h *Handlers) SiteAutomationsUnarchive(ctx context.Context, params siteapi.SiteAutomationsUnarchiveParams) (*siteapi.SiteAutomationResource, error) {
	ws := workspaceScope(ctx)

	id := int64(params.ID)
	a, err := h.automations.Unarchive(ctx, ws, id)
	if err != nil {
		return nil, err
	}
	res, err := mapper.AutomationToResource(a)
	if err != nil {
		return nil, err
	}
	return &res, nil
}

// SiteAutomationsPublish makes the Automation active: it starts enrolling Contacts.
func (h *Handlers) SiteAutomationsPublish(ctx context.Context, req *siteapi.RevisionInput, params siteapi.SiteAutomationsPublishParams) (*siteapi.SiteAutomationResource, error) {
	ws := workspaceScope(ctx)
	a, err := h.automations.Publish(ctx, ws, int64(params.ID), int64(req.Revision))
	if err != nil {
		return nil, err
	}
	res, err := mapper.AutomationToResource(a)
	if err != nil {
		return nil, err
	}
	return &res, nil
}

// SiteAutomationsDeactivate makes the Automation inactive: it stops new Enrollments.
func (h *Handlers) SiteAutomationsDeactivate(ctx context.Context, params siteapi.SiteAutomationsDeactivateParams) (*siteapi.SiteAutomationResource, error) {
	ws := workspaceScope(ctx)
	a, err := h.automations.Deactivate(ctx, ws, int64(params.ID))
	if err != nil {
		return nil, err
	}
	res, err := mapper.AutomationToResource(a)
	if err != nil {
		return nil, err
	}
	return &res, nil
}

// SiteAutomationsRevert drops the Automation's draft: the live version is again the latest.
func (h *Handlers) SiteAutomationsRevert(ctx context.Context, req *siteapi.RevisionInput, params siteapi.SiteAutomationsRevertParams) (*siteapi.SiteAutomationResource, error) {
	ws := workspaceScope(ctx)
	a, err := h.automations.Revert(ctx, ws, int64(params.ID), int64(req.Revision))
	if err != nil {
		return nil, err
	}
	res, err := mapper.AutomationToResource(a)
	if err != nil {
		return nil, err
	}
	return &res, nil
}
