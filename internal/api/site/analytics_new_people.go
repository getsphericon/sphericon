package site

import (
	"context"
	"net/http"

	"github.com/getsphericon/sphericon/ent"
	siteapi "github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/narrow"
)

// SiteAnalyticsNewPeople maps the New people report onto the DTO.
func (h *Handlers) SiteAnalyticsNewPeople(ctx context.Context, params siteapi.SiteAnalyticsNewPeopleParams) (siteapi.SiteAnalyticsNewPeopleRes, error) {
	s, err := h.scopedFor(ctx, params.WorkspaceId)
	if ent.IsNotFound(err) {
		v := problem(http.StatusNotFound, "workspace not found")
		return &v, nil
	}
	if err != nil {
		return nil, err
	}
	report, err := h.analytics.NewPeople(ctx, s, analyticsWindow(params.Range))
	if err != nil {
		return nil, err
	}
	series := make([]siteapi.SiteNewPeoplePoint, len(report.Series))
	for i, p := range report.Series {
		series[i] = siteapi.SiteNewPeoplePoint{
			Date: p.Date, Manual: narrow.Int32(p.Manual), API: narrow.Int32(p.API),
			Tracker: narrow.Int32(p.Tracker), Unknown: narrow.Int32(p.Unknown),
		}
	}
	return &siteapi.SiteNewPeopleReport{Total: narrow.Int32(report.Total), Timeseries: series}, nil
}
