package external

import (
	"context"

	externalapi "github.com/getsphericon/sphericon/gen/external"
	"github.com/getsphericon/sphericon/internal/api/auth"
	"github.com/getsphericon/sphericon/internal/automations"
	"github.com/getsphericon/sphericon/internal/narrow"
	"github.com/getsphericon/sphericon/internal/pagination"
)

func (h *Handlers) AutomationsList(ctx context.Context, params externalapi.AutomationsListParams) (externalapi.AutomationsListRes, error) {
	page, err := h.automations.List(ctx, auth.TokenScoped(ctx), pagination.ParamsOf(params.Page, params.PageSize), params.Archived.Or(false))
	if err != nil {
		return nil, err
	}
	return &externalapi.AutomationsListOK{
		Items:      mapper.AutomationsToResources(page.Items),
		Page:       narrow.Int32(page.Page),
		PageSize:   narrow.Int32(page.PageSize),
		TotalItems: narrow.Int32(page.TotalItems),
		TotalPages: narrow.Int32(page.TotalPages),
	}, nil
}

// AutomationsCreate always stores an inactive Automation: publishing is the send-class operation.
func (h *Handlers) AutomationsCreate(ctx context.Context, req *externalapi.CreateAutomationInput) (externalapi.AutomationsCreateRes, error) {
	a, err := h.automations.Create(ctx, auth.TokenScoped(ctx), inputs.AutomationCreate(req))
	if err != nil {
		return nil, err
	}
	res := mapper.AutomationToResource(a)
	return &res, nil
}

func (h *Handlers) AutomationsGet(ctx context.Context, params externalapi.AutomationsGetParams) (externalapi.AutomationsGetRes, error) {
	id := int64(params.ID)
	a, err := h.automations.Get(ctx, auth.TokenScoped(ctx), id)
	if err != nil {
		return nil, err
	}
	res := mapper.AutomationToResource(a)
	return &res, nil
}

func (h *Handlers) AutomationsUpdate(ctx context.Context, req *externalapi.UpdateAutomationInput, params externalapi.AutomationsUpdateParams) (externalapi.AutomationsUpdateRes, error) {
	id := int64(params.ID)
	a, err := h.automations.Update(ctx, auth.TokenScoped(ctx), id, inputs.AutomationUpdate(req))
	if err != nil {
		return nil, err
	}
	res := mapper.AutomationToResource(a)
	return &res, nil
}

func (h *Handlers) AutomationsDelete(ctx context.Context, params externalapi.AutomationsDeleteParams) (externalapi.AutomationsDeleteRes, error) {
	id := int64(params.ID)
	err := h.automations.Delete(ctx, auth.TokenScoped(ctx), id)
	if err != nil {
		return nil, err
	}
	return &externalapi.AutomationsDeleteNoContent{}, nil
}

// AutomationsArchive archives an automation (ADR 0049); active enrollments refuse it with 409.
func (h *Handlers) AutomationsArchive(ctx context.Context, params externalapi.AutomationsArchiveParams) (externalapi.AutomationsArchiveRes, error) {
	ws := auth.TokenScoped(ctx)
	id := int64(params.ID)
	a, err := h.automations.Archive(ctx, ws, id)
	if err != nil {
		return nil, err
	}
	res := mapper.AutomationToResource(a)
	return &res, nil
}

// AutomationsUnarchive restores an archived automation.
func (h *Handlers) AutomationsUnarchive(ctx context.Context, params externalapi.AutomationsUnarchiveParams) (externalapi.AutomationsUnarchiveRes, error) {
	ws := auth.TokenScoped(ctx)
	id := int64(params.ID)
	a, err := h.automations.Unarchive(ctx, ws, id)
	if err != nil {
		return nil, err
	}
	res := mapper.AutomationToResource(a)
	return &res, nil
}

// AutomationsPublish needs automations:publish, which authoring scopes never imply (ADR 0016,
// ADR 0065). Over MCP, it additionally needs mcp:send (the x-mcp send flag).
func (h *Handlers) AutomationsPublish(ctx context.Context, params externalapi.AutomationsPublishParams) (externalapi.AutomationsPublishRes, error) {
	id := int64(params.ID)
	a, err := h.automations.Publish(ctx, auth.TokenScoped(ctx), id)
	if err != nil {
		return nil, err
	}
	res := mapper.AutomationToResource(a)
	return &res, nil
}

// AutomationsDeactivate only stops new Enrollments, so it is authoring: automations:write.
func (h *Handlers) AutomationsDeactivate(ctx context.Context, params externalapi.AutomationsDeactivateParams) (externalapi.AutomationsDeactivateRes, error) {
	id := int64(params.ID)
	a, err := h.automations.Deactivate(ctx, auth.TokenScoped(ctx), id)
	if err != nil {
		return nil, err
	}
	res := mapper.AutomationToResource(a)
	return &res, nil
}

// AutomationsReport reads the engagement of the Automation's emails and its results over the
// period the request names; the contract's default fills it in when the request names none.
func (h *Handlers) AutomationsReport(ctx context.Context, params externalapi.AutomationsReportParams) (externalapi.AutomationsReportRes, error) {
	period, err := automations.PeriodOf(params.Period.Value)
	if err != nil {
		return nil, err
	}
	report, err := h.automations.ReportOf(ctx, auth.TokenScoped(ctx), int64(params.ID), period)
	if err != nil {
		return nil, err
	}
	res := mapper.AutomationReportToResource(report)
	res.Results.Period = externalapi.AutomationReportPeriod(params.Period.Value)
	return &res, nil
}

// AutomationsRevert drops the draft and changes nothing that sends, so it is authoring:
// automations:write.
func (h *Handlers) AutomationsRevert(ctx context.Context, params externalapi.AutomationsRevertParams) (externalapi.AutomationsRevertRes, error) {
	a, err := h.automations.Revert(ctx, auth.TokenScoped(ctx), int64(params.ID))
	if err != nil {
		return nil, err
	}
	res := mapper.AutomationToResource(a)
	return &res, nil
}
