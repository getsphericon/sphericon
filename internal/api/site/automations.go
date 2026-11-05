package site

import (
	"context"

	siteapi "github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/pagination"
)

func (h *Handlers) SiteAutomationsList(ctx context.Context, params siteapi.SiteAutomationsListParams) (*siteapi.SiteAutomationsListOK, error) {
	ws := workspaceScope(ctx)

	page, err := h.automations.ListWithRevenue(ctx, ws, pagination.ParamsOf(params.Page, params.PageSize), inputs.AutomationsListFilter(params))
	if err != nil {
		return nil, err
	}
	return mapper.AutomationsListPage(page), nil
}

func (h *Handlers) SiteAutomationsCreate(ctx context.Context, req *siteapi.SiteCreateAutomationInput, _ siteapi.SiteAutomationsCreateParams) (*siteapi.SiteAutomationResource, error) {
	ws := workspaceScope(ctx)

	a, err := h.automations.Create(ctx, ws, inputs.AutomationCreate(req))
	if err != nil {
		return nil, err
	}
	res := mapper.AutomationToResource(a)
	return &res, nil
}

func (h *Handlers) SiteAutomationsGet(ctx context.Context, params siteapi.SiteAutomationsGetParams) (*siteapi.SiteAutomationResource, error) {
	ws := workspaceScope(ctx)

	a, err := h.automations.GetWithRevenue(ctx, ws, int64(params.ID))
	if err != nil {
		return nil, err
	}
	res := mapper.AutomationWithRevenueToResource(a)
	return &res, nil
}

func (h *Handlers) SiteAutomationsUpdate(ctx context.Context, req *siteapi.SiteUpdateAutomationInput, params siteapi.SiteAutomationsUpdateParams) (*siteapi.SiteAutomationResource, error) {
	ws := workspaceScope(ctx)

	id := int64(params.ID)
	a, err := h.automations.Update(ctx, ws, id, inputs.AutomationUpdate(req))
	if err != nil {
		return nil, err
	}
	res := mapper.AutomationToResource(a)
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
	res := mapper.AutomationToResource(a)
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
	res := mapper.AutomationToResource(a)
	return &res, nil
}

// SiteAutomationsPublish makes the Automation active: it starts enrolling Contacts.
func (h *Handlers) SiteAutomationsPublish(ctx context.Context, params siteapi.SiteAutomationsPublishParams) (*siteapi.SiteAutomationResource, error) {
	ws := workspaceScope(ctx)
	a, err := h.automations.Publish(ctx, ws, int64(params.ID))
	if err != nil {
		return nil, err
	}
	res := mapper.AutomationToResource(a)
	return &res, nil
}

// SiteAutomationsDeactivate makes the Automation inactive: it stops new Enrollments.
func (h *Handlers) SiteAutomationsDeactivate(ctx context.Context, params siteapi.SiteAutomationsDeactivateParams) (*siteapi.SiteAutomationResource, error) {
	ws := workspaceScope(ctx)
	a, err := h.automations.Deactivate(ctx, ws, int64(params.ID))
	if err != nil {
		return nil, err
	}
	res := mapper.AutomationToResource(a)
	return &res, nil
}

// SiteAutomationsRevert drops the Automation's draft: the live version is again the latest.
func (h *Handlers) SiteAutomationsRevert(ctx context.Context, params siteapi.SiteAutomationsRevertParams) (*siteapi.SiteAutomationResource, error) {
	ws := workspaceScope(ctx)
	a, err := h.automations.Revert(ctx, ws, int64(params.ID))
	if err != nil {
		return nil, err
	}
	res := mapper.AutomationToResource(a)
	return &res, nil
}
