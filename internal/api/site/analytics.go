package site

import (
	"context"
	"net/http"

	"github.com/getsphericon/sphericon/ent"
	siteapi "github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/analytics"
	"github.com/getsphericon/sphericon/internal/narrow"
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
		case siteapi.SiteAnalyticsRange30d:
			return analytics.Window30Days
		}
	}
	return analytics.Window30Days
}

// SiteAnalyticsOverview maps the analytics module's Overview onto the DTO.
func (h *Handlers) SiteAnalyticsOverview(ctx context.Context, params siteapi.SiteAnalyticsOverviewParams) (siteapi.SiteAnalyticsOverviewRes, error) {
	s, err := h.scopedFor(ctx, params.WorkspaceId)
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
	warning, err := h.abuseWarning(ctx, s)
	if err != nil {
		return nil, err
	}
	series := make([]siteapi.SiteAnalyticsPoint, len(ov.Series))
	for i, p := range ov.Series {
		series[i] = siteapi.SiteAnalyticsPoint{Date: p.Date, Sent: narrow.Int32(p.Sent), Opened: narrow.Int32(p.Opened), Clicked: narrow.Int32(p.Clicked)}
	}
	return &siteapi.SiteAnalyticsOverview{
		Contacts: siteapi.SiteAnalyticsContacts{
			Total: narrow.Int32(ov.Contacts.Total), Active: narrow.Int32(ov.Contacts.Active),
			Unsubscribed: narrow.Int32(ov.Contacts.Unsubscribed), NewInRange: narrow.Int32(ov.Contacts.NewInWindow),
		},
		Email: siteapi.SiteAnalyticsEmail{
			SentCount: narrow.Int32(ov.Email.Sent), OpenedCount: narrow.Int32(ov.Email.Opened), ClickedCount: narrow.Int32(ov.Email.Clicked),
			OpenRate: ov.Email.OpenRate, ClickRate: ov.Email.ClickRate, ClickToOpenRate: ov.Email.ClickToOpenRate,
		},
		Automations: siteapi.SiteAnalyticsAutomations{
			Total: narrow.Int32(ov.Automations.Total), Active: narrow.Int32(ov.Automations.Active),
			RunsActive: narrow.Int32(ov.Automations.RunsActive), RunsCompleted: narrow.Int32(ov.Automations.RunsCompleted),
		},
		Timeseries:   series,
		AbuseWarning: warning,
	}, nil
}

// abuseWarning is the Workspace's place in the abuse detector's warning tier, computed
// live (ADR 0026); unset without the detector.
func (h *Handlers) abuseWarning(ctx context.Context, s *ent.Scoped) (siteapi.OptNilSiteAbuseWarning, error) {
	if h.abuseWatch == nil {
		return siteapi.OptNilSiteAbuseWarning{}, nil
	}
	ws, err := s.Workspace(ctx)
	if err != nil {
		return siteapi.OptNilSiteAbuseWarning{}, err
	}
	w, err := h.abuseWatch.Warning(ctx, s, ws, h.now())
	if err != nil || w == nil {
		return siteapi.OptNilSiteAbuseWarning{}, err
	}
	return siteapi.NewOptNilSiteAbuseWarning(siteapi.SiteAbuseWarning{
		Domain: w.Domain, Metric: siteapi.SiteAbuseMetric(w.Metric), Rate: w.Rate,
	}), nil
}

// SiteAnalyticsSubscribedPeople maps the Subscribed people report onto the DTO.
func (h *Handlers) SiteAnalyticsSubscribedPeople(ctx context.Context, params siteapi.SiteAnalyticsSubscribedPeopleParams) (siteapi.SiteAnalyticsSubscribedPeopleRes, error) {
	s, err := h.scopedFor(ctx, params.WorkspaceId)
	if ent.IsNotFound(err) {
		v := problem(http.StatusNotFound, "workspace not found")
		return &v, nil
	}
	if err != nil {
		return nil, err
	}
	report, err := h.analytics.SubscribedPeople(ctx, s, analyticsWindow(params.Range))
	if err != nil {
		return nil, err
	}
	series := make([]siteapi.SiteAnalyticsSubscribedPoint, len(report.Series))
	for i, p := range report.Series {
		series[i] = siteapi.SiteAnalyticsSubscribedPoint{
			Date: p.Date, Subscribed: narrow.Int32(p.Subscribed), Unsubscribed: narrow.Int32(p.Unsubscribed), Net: narrow.Int32(p.Net),
		}
	}
	return &siteapi.SiteAnalyticsSubscribedPeople{Current: narrow.Int32(report.Current), Series: series}, nil
}
