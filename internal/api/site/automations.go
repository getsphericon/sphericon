package site

import (
	"context"

	"github.com/samber/lo"

	"github.com/getsphericon/sphericon/ent"
	siteapi "github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/api/site/resources"
	"github.com/getsphericon/sphericon/internal/automations"
	"github.com/getsphericon/sphericon/internal/convert"
	"github.com/getsphericon/sphericon/internal/narrow"
	"github.com/getsphericon/sphericon/internal/orders"
	"github.com/getsphericon/sphericon/internal/pagination"
)

func (h *Handlers) SiteAutomationsList(ctx context.Context, params siteapi.SiteAutomationsListParams) (siteapi.SiteAutomationsListRes, error) {
	ws := workspaceScope(ctx)

	page, err := h.automations.List(ctx, ws, pagination.ParamsOf(params.Page, params.PageSize), params.Archived.Or(false))
	if err != nil {
		return nil, err
	}

	revenue, err := orders.CreditedToAutomations(ctx, ws, lo.Map(page.Items, func(a *ent.Automation, _ int) int64 { return a.ID })...)
	if err != nil {
		return nil, err
	}
	return &siteapi.SiteAutomationsListOK{
		Items: lo.Map(page.Items, func(a *ent.Automation, _ int) siteapi.SiteAutomationResource {
			res := mapper.AutomationToResource(a)
			res.Revenue = siteapi.NewOptSiteMessageRevenue(resources.MessageRevenue(revenue.Only(a.ID)))
			return res
		}),
		Page:       narrow.Int32(page.Page),
		PageSize:   narrow.Int32(page.PageSize),
		TotalItems: narrow.Int32(page.TotalItems),
		TotalPages: narrow.Int32(page.TotalPages),
	}, nil
}

func (h *Handlers) SiteAutomationsCreate(ctx context.Context, req *siteapi.SiteCreateAutomationInput, _ siteapi.SiteAutomationsCreateParams) (siteapi.SiteAutomationsCreateRes, error) {
	ws := workspaceScope(ctx)

	in := automations.CreateInput{Name: req.Name, TriggerEvent: req.TriggerEvent, IntegrationID: convert.Int64Ptr(req.IntegrationId)}
	if req.Steps != nil {
		in.Steps = resources.AutomationSteps(req.Steps)
	}
	a, err := h.automations.Create(ctx, ws, in)
	if err != nil {
		return nil, err
	}
	res := mapper.AutomationToResource(a)
	return &res, nil
}

func (h *Handlers) SiteAutomationsGet(ctx context.Context, params siteapi.SiteAutomationsGetParams) (siteapi.SiteAutomationsGetRes, error) {
	ws := workspaceScope(ctx)

	id := int64(params.ID)
	a, err := h.automations.Get(ctx, ws, id)
	if err != nil {
		return nil, err
	}
	revenue, err := orders.CreditedToAutomations(ctx, ws, a.ID)
	if err != nil {
		return nil, err
	}
	res := mapper.AutomationToResource(a)
	res.Revenue = siteapi.NewOptSiteMessageRevenue(resources.MessageRevenue(revenue.Only(a.ID)))
	return &res, nil
}

func (h *Handlers) SiteAutomationsUpdate(ctx context.Context, req *siteapi.SiteUpdateAutomationInput, params siteapi.SiteAutomationsUpdateParams) (siteapi.SiteAutomationsUpdateRes, error) {
	ws := workspaceScope(ctx)

	id := int64(params.ID)
	in := automations.UpdateInput{
		Name:         convert.StringPtr(req.Name),
		TriggerEvent: convert.StringPtr(req.TriggerEvent),
		// JSON Merge Patch: an explicit null clears the Integration, an absent key keeps it.
		IntegrationID:    convert.Int64Ptr(req.IntegrationId),
		ClearIntegration: req.IntegrationId.IsNull(),
	}
	if req.Steps != nil {
		steps := resources.AutomationSteps(req.Steps)
		in.Steps = &steps
	}
	a, err := h.automations.Update(ctx, ws, id, in)
	if err != nil {
		return nil, err
	}
	res := mapper.AutomationToResource(a)
	return &res, nil
}

func (h *Handlers) SiteAutomationsDelete(ctx context.Context, params siteapi.SiteAutomationsDeleteParams) (siteapi.SiteAutomationsDeleteRes, error) {
	ws := workspaceScope(ctx)

	id := int64(params.ID)
	err := h.automations.Delete(ctx, ws, id)
	if err != nil {
		return nil, err
	}
	return &siteapi.SiteAutomationsDeleteNoContent{}, nil
}

// SiteAutomationsArchive archives an automation (ADR 0049): it leaves the list and enrolls
// nobody, and keeps its enrollments. Active enrollments refuse it with 409.
func (h *Handlers) SiteAutomationsArchive(ctx context.Context, params siteapi.SiteAutomationsArchiveParams) (siteapi.SiteAutomationsArchiveRes, error) {
	ws := workspaceScope(ctx)

	id := int64(params.ID)
	err := h.automations.Archive(ctx, ws, id)
	if err != nil {
		return nil, err
	}
	a, err := h.automations.Get(ctx, ws, id)
	if err != nil {
		return nil, err
	}
	res := mapper.AutomationToResource(a)
	return &res, nil
}

// SiteAutomationsUnarchive restores an archived automation to the list.
func (h *Handlers) SiteAutomationsUnarchive(ctx context.Context, params siteapi.SiteAutomationsUnarchiveParams) (siteapi.SiteAutomationsUnarchiveRes, error) {
	ws := workspaceScope(ctx)

	id := int64(params.ID)
	err := h.automations.Unarchive(ctx, ws, id)
	if err != nil {
		return nil, err
	}
	a, err := h.automations.Get(ctx, ws, id)
	if err != nil {
		return nil, err
	}
	res := mapper.AutomationToResource(a)
	return &res, nil
}

// SiteAutomationsPublish makes the Automation active: it starts enrolling Contacts.
func (h *Handlers) SiteAutomationsPublish(ctx context.Context, params siteapi.SiteAutomationsPublishParams) (siteapi.SiteAutomationsPublishRes, error) {
	ws := workspaceScope(ctx)
	a, err := h.automations.Publish(ctx, ws, int64(params.ID))
	if err != nil {
		return nil, err
	}
	res := mapper.AutomationToResource(a)
	return &res, nil
}

// SiteAutomationsDeactivate makes the Automation inactive: it stops new Enrollments.
func (h *Handlers) SiteAutomationsDeactivate(ctx context.Context, params siteapi.SiteAutomationsDeactivateParams) (siteapi.SiteAutomationsDeactivateRes, error) {
	ws := workspaceScope(ctx)
	a, err := h.automations.Deactivate(ctx, ws, int64(params.ID))
	if err != nil {
		return nil, err
	}
	res := mapper.AutomationToResource(a)
	return &res, nil
}

// SiteAutomationsRevert drops the Automation's draft: the live version is again the latest.
func (h *Handlers) SiteAutomationsRevert(ctx context.Context, params siteapi.SiteAutomationsRevertParams) (siteapi.SiteAutomationsRevertRes, error) {
	ws := workspaceScope(ctx)
	a, err := h.automations.Revert(ctx, ws, int64(params.ID))
	if err != nil {
		return nil, err
	}
	res := mapper.AutomationToResource(a)
	return &res, nil
}
