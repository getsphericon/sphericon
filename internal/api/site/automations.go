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
	"github.com/getsphericon/sphericon/internal/pagination"
)

func (h *Handlers) SiteAutomationsList(ctx context.Context, params siteapi.SiteAutomationsListParams) (siteapi.SiteAutomationsListRes, error) {
	ws, err := h.scopedFor(ctx, params.WorkspaceId)
	if err != nil {
		return nil, err
	}

	page, err := h.automations.List(ctx, ws, pagination.ParamsOf(params.Page, params.PageSize), params.Archived.Or(false))
	if err != nil {
		return nil, err
	}

	return &siteapi.SiteAutomationsListOK{
		Items:      lo.Map(page.Items, func(a *ent.Automation, _ int) siteapi.SiteAutomationResource { return mapper.AutomationToResource(a) }),
		Page:       narrow.Int32(page.Page),
		PageSize:   narrow.Int32(page.PageSize),
		TotalItems: narrow.Int32(page.TotalItems),
		TotalPages: narrow.Int32(page.TotalPages),
	}, nil
}

func (h *Handlers) SiteAutomationsCreate(ctx context.Context, req *siteapi.SiteCreateAutomationInput, params siteapi.SiteAutomationsCreateParams) (siteapi.SiteAutomationsCreateRes, error) {
	ws, err := h.scopedFor(ctx, params.WorkspaceId)
	if err != nil {
		return nil, err
	}

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
	ws, err := h.scopedFor(ctx, params.WorkspaceId)
	if err != nil {
		return nil, err
	}

	id := int64(params.ID)
	a, err := h.automations.Get(ctx, ws, id)
	if err != nil {
		return nil, err
	}
	res := mapper.AutomationToResource(a)
	return &res, nil
}

func (h *Handlers) SiteAutomationsUpdate(ctx context.Context, req *siteapi.SiteUpdateAutomationInput, params siteapi.SiteAutomationsUpdateParams) (siteapi.SiteAutomationsUpdateRes, error) {
	ws, err := h.scopedFor(ctx, params.WorkspaceId)
	if err != nil {
		return nil, err
	}

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
	ws, err := h.scopedFor(ctx, params.WorkspaceId)
	if err != nil {
		return nil, err
	}

	id := int64(params.ID)
	err = h.automations.Delete(ctx, ws, id)
	if err != nil {
		return nil, err
	}
	return &siteapi.SiteAutomationsDeleteNoContent{}, nil
}

// SiteAutomationsArchive archives an automation (ADR 0049): it leaves the list and enrolls
// nobody, and keeps its enrollments. Active enrollments refuse it with 409.
func (h *Handlers) SiteAutomationsArchive(ctx context.Context, params siteapi.SiteAutomationsArchiveParams) (siteapi.SiteAutomationsArchiveRes, error) {
	ws, err := h.scopedFor(ctx, params.WorkspaceId)
	if err != nil {
		return nil, err
	}

	id := int64(params.ID)
	err = h.automations.Archive(ctx, ws, id)
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
	ws, err := h.scopedFor(ctx, params.WorkspaceId)
	if err != nil {
		return nil, err
	}

	id := int64(params.ID)
	err = h.automations.Unarchive(ctx, ws, id)
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

func (h *Handlers) SiteAutomationsActivate(ctx context.Context, params siteapi.SiteAutomationsActivateParams) (siteapi.SiteAutomationsActivateRes, error) {
	a, err := h.setAutomationActive(ctx, params.WorkspaceId, params.ID, true)
	if err != nil {
		return nil, err
	}
	res := mapper.AutomationToResource(a)
	return &res, nil
}

func (h *Handlers) SiteAutomationsDeactivate(ctx context.Context, params siteapi.SiteAutomationsDeactivateParams) (siteapi.SiteAutomationsDeactivateRes, error) {
	a, err := h.setAutomationActive(ctx, params.WorkspaceId, params.ID, false)
	if err != nil {
		return nil, err
	}
	res := mapper.AutomationToResource(a)
	return &res, nil
}

func (h *Handlers) setAutomationActive(ctx context.Context, workspaceID siteapi.WorkspaceId, id siteapi.EntityId, active bool) (*ent.Automation, error) {
	ws, err := h.scopedFor(ctx, workspaceID)
	if err != nil {
		return nil, err
	}
	parsed := int64(id)
	if active {
		return h.automations.Activate(ctx, ws, parsed)
	}
	return h.automations.Deactivate(ctx, ws, parsed)
}
