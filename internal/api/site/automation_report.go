package site

import (
	"context"

	siteapi "github.com/getsphericon/sphericon/gen/site"
)

// SiteAutomationReportsGet reads the report over the period the request names; the contract's
// default fills it in when the request names none.
func (h *Handlers) SiteAutomationReportsGet(ctx context.Context, params siteapi.SiteAutomationReportsGetParams) (siteapi.SiteAutomationReportsGetRes, error) {
	period := inputs.AutomationReportPeriod(params.Period.Value)
	report, err := h.automations.ReportOf(ctx, workspaceScope(ctx), int64(params.AutomationId), period)
	if err != nil {
		return nil, err
	}
	return new(mapper.AutomationReportToResource(report)), nil
}
