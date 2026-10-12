package site

import (
	"context"
	"net/http"

	"github.com/getsphericon/sphericon/ent"
	siteapi "github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/analytics"
)

// analyticsWindow maps the selectable range to the module's window; 30d is the
// default when the param is absent or unrecognized.
func analyticsWindow(r siteapi.OptSiteAnalyticsRange) analytics.Window {
	if v, ok := r.Get(); ok {
		switch v {
		case siteapi.SiteAnalyticsRange7d:
			return analytics.Window7Days
		case siteapi.SiteAnalyticsRange90d:
			return analytics.Window90Days
		}
	}
	return analytics.Window30Days
}

// SiteAnalyticsOverview maps the analytics module's Overview onto the DTO.
func (h *Handlers) SiteAnalyticsOverview(ctx context.Context, params siteapi.SiteAnalyticsOverviewParams) (siteapi.SiteAnalyticsOverviewRes, error) {
	s, err := h.scopedFor(ctx, params.Slug)
	if ent.IsNotFound(err) {
		v := problem(http.StatusNotFound, "workspace not found")
		return &v, nil
	}
	if err != nil {
		return nil, err
	}
	ov, err := h.analytics.Overview(ctx, s, analyticsWindow(params.Range))
	if err != nil {
		return nil, err
	}
	series := make([]siteapi.SiteAnalyticsPoint, len(ov.Series))
	for i, p := range ov.Series {
		series[i] = siteapi.SiteAnalyticsPoint{Date: p.Date, Sent: int32(p.Sent), Opened: int32(p.Opened), Clicked: int32(p.Clicked)}
	}
	return &siteapi.SiteAnalyticsOverview{
		Contacts: siteapi.SiteAnalyticsContacts{
			Total: int32(ov.Contacts.Total), Active: int32(ov.Contacts.Active),
			Unsubscribed: int32(ov.Contacts.Unsubscribed), NewInRange: int32(ov.Contacts.NewInWindow),
		},
		Email: siteapi.SiteAnalyticsEmail{
			SentCount: int32(ov.Email.Sent), OpenedCount: int32(ov.Email.Opened), ClickedCount: int32(ov.Email.Clicked),
			OpenRate: ov.Email.OpenRate, ClickRate: ov.Email.ClickRate, ClickToOpenRate: ov.Email.ClickToOpenRate,
		},
		Automations: siteapi.SiteAnalyticsAutomations{
			Total: int32(ov.Automations.Total), Active: int32(ov.Automations.Active),
			RunsActive: int32(ov.Automations.RunsActive), RunsCompleted: int32(ov.Automations.RunsCompleted),
		},
		Timeseries: series,
	}, nil
}
