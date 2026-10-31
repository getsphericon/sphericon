package site

import (
	"context"

	siteapi "github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/automations"
)

// SiteAutomationReportsGet reads the report over the period the request names; the contract's
// default fills it in when the request names none.
func (h *Handlers) SiteAutomationReportsGet(ctx context.Context, params siteapi.SiteAutomationReportsGetParams) (siteapi.SiteAutomationReportsGetRes, error) {
	period, err := automations.PeriodOf(params.Period.Value)
	if err != nil {
		return nil, err
	}
	report, err := h.automations.ReportOf(ctx, workspaceScope(ctx), int64(params.AutomationId), period)
	if err != nil {
		return nil, err
	}
	res := mapper.AutomationReportToResource(report)
	res.Results.Period = siteapi.SiteAutomationReportPeriod(params.Period.Value)
	return &res, nil
}
