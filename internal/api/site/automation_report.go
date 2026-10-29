package site

import (
	"context"
	"errors"
	"net/http"

	siteapi "github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/automations"
	"github.com/getsphericon/sphericon/internal/narrow"
)

func (h *Handlers) SiteAutomationReportsGet(ctx context.Context, params siteapi.SiteAutomationReportsGetParams) (siteapi.SiteAutomationReportsGetRes, error) {
	ws, err := h.scopedFor(ctx, params.WorkspaceId)
	if err != nil {
		return nil, err
	}
	report, err := h.automations.ReportOf(ctx, ws, int64(params.AutomationId))
	if errors.Is(err, automations.ErrNotFound) {
		v := siteapi.SiteAutomationReportsGetNotFound(problem(http.StatusNotFound, "automation not found"))
		return &v, nil
	}
	if err != nil {
		return nil, err
	}
	return &siteapi.SiteAutomationReport{
		Opens:      narrow.Int32(report.Opens),
		Clicks:     narrow.Int32(report.Clicks),
		Delivered:  narrow.Int32(report.Delivered),
		Bounced:    narrow.Int32(report.Bounced),
		Complained: narrow.Int32(report.Complained),
		Captured:   narrow.Int32(report.Captured),
		Links:      mapper.LinkClicksToResource(report.Links),
	}, nil
}
