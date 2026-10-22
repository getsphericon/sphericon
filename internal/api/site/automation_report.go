package site

import (
	"context"
	"errors"
	"net/http"

	"github.com/getsphericon/sphericon/ent"
	siteapi "github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/automations"
)

func (h *Handlers) SiteAutomationReportsGet(ctx context.Context, params siteapi.SiteAutomationReportsGetParams) (siteapi.SiteAutomationReportsGetRes, error) {
	ws, err := h.scopedFor(ctx, params.WorkspaceId)
	if ent.IsNotFound(err) {
		v := siteapi.SiteAutomationReportsGetNotFound(problem(http.StatusNotFound, "workspace not found"))
		return &v, nil
	}
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
		Opens:      int32(report.Opens),
		Clicks:     int32(report.Clicks),
		Delivered:  int32(report.Delivered),
		Bounced:    int32(report.Bounced),
		Complained: int32(report.Complained),
		Links:      mapper.LinkClicksToResource(report.Links),
	}, nil
}
