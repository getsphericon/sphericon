package site

import (
	"context"

	"github.com/getsphericon/sphericon/ent"
	siteapi "github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/analytics"
	"github.com/getsphericon/sphericon/internal/narrow"
)

// analyticsWindow maps the selectable range to the module's window; 7d is the
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
	return analytics.Window7Days
}

// analyticsReport is the shell every report handler shares: it reads the selected window and hands
// it, with the scope of the Workspace of the path, to build, which maps the module's report onto
// the operation's response.
func analyticsReport[Res any](ctx context.Context, rng siteapi.OptSiteAnalyticsRange, build func(*ent.Scoped, analytics.Window) (Res, error)) (Res, error) {
	return build(workspaceScope(ctx), analyticsWindow(rng))
}

// SiteAnalyticsOverview maps the analytics module's Overview onto the DTO.
func (h *Handlers) SiteAnalyticsOverview(ctx context.Context, params siteapi.SiteAnalyticsOverviewParams) (siteapi.SiteAnalyticsOverviewRes, error) {
	return analyticsReport(ctx, params.Range, func(s *ent.Scoped, w analytics.Window) (siteapi.SiteAnalyticsOverviewRes, error) {
		ov, err := h.analytics.Overview(ctx, s, w)
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
	})
}

// SiteAnalyticsEvents maps the analytics module's Events report onto the DTO.
func (h *Handlers) SiteAnalyticsEvents(ctx context.Context, params siteapi.SiteAnalyticsEventsParams) (siteapi.SiteAnalyticsEventsRes, error) {
	return analyticsReport(ctx, params.Range, func(s *ent.Scoped, w analytics.Window) (siteapi.SiteAnalyticsEventsRes, error) {
		rows, err := h.analytics.Events(ctx, s, w)
		if err != nil {
			return nil, err
		}
		actions := make([]siteapi.SiteAnalyticsEventAction, len(rows))
		for i, r := range rows {
			actions[i] = siteapi.SiteAnalyticsEventAction{
				Action: r.Action, Count: narrow.Int32(r.Count), Contacts: narrow.Int32(r.Contacts), LastAt: siteapi.Timestamp(r.LastAt),
			}
		}
		return &siteapi.SiteAnalyticsEvents{Actions: actions}, nil
	})
}

// abuseWarning is the Workspace's place in the abuse detector's warning tier, computed
// live (ADR 0026); unset without the detector.
func (h *Handlers) abuseWarning(ctx context.Context, s *ent.Scoped) (siteapi.OptNilSiteAbuseWarning, error) {
	if h.abuseWatch == nil {
		return siteapi.OptNilSiteAbuseWarning{}, nil
	}
	w, err := h.abuseWatch.Warning(ctx, s, h.now())
	if err != nil || w == nil {
		return siteapi.OptNilSiteAbuseWarning{}, err
	}
	return siteapi.NewOptNilSiteAbuseWarning(siteapi.SiteAbuseWarning{
		Domain: w.Domain, Metric: siteapi.SiteAbuseMetric(w.Metric), Rate: w.Rate,
	}), nil
}

// SiteAnalyticsSubscribedPeople maps the Subscribed people report onto the DTO.
func (h *Handlers) SiteAnalyticsSubscribedPeople(ctx context.Context, params siteapi.SiteAnalyticsSubscribedPeopleParams) (siteapi.SiteAnalyticsSubscribedPeopleRes, error) {
	return analyticsReport(ctx, params.Range, func(s *ent.Scoped, w analytics.Window) (siteapi.SiteAnalyticsSubscribedPeopleRes, error) {
		report, err := h.analytics.SubscribedPeople(ctx, s, w)
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
	})
}

// SiteAnalyticsClicksByLink maps the analytics module's Clicks by link report onto the DTO.
func (h *Handlers) SiteAnalyticsClicksByLink(ctx context.Context, params siteapi.SiteAnalyticsClicksByLinkParams) (siteapi.SiteAnalyticsClicksByLinkRes, error) {
	return analyticsReport(ctx, params.Range, func(s *ent.Scoped, w analytics.Window) (siteapi.SiteAnalyticsClicksByLinkRes, error) {
		links, err := h.analytics.ClicksByLink(ctx, s, w)
		if err != nil {
			return nil, err
		}
		out := make([]siteapi.SiteAnalyticsLink, len(links))
		for i, l := range links {
			owners := make([]siteapi.SiteAnalyticsLinkOwner, len(l.Owners))
			for j, o := range l.Owners {
				owners[j] = siteapi.SiteAnalyticsLinkOwner{
					Kind: siteapi.SiteAnalyticsOwnerKind(o.Kind), ID: siteapi.EntityId(o.ID), Name: o.Name, Clicks: narrow.Int32(o.Clicks),
				}
			}
			out[i] = siteapi.SiteAnalyticsLink{URL: l.URL, Clicks: narrow.Int32(l.Clicks), Owners: owners}
		}
		return &siteapi.SiteAnalyticsClicksByLink{Links: out, EventsRetentionDays: narrow.Int32(h.analytics.RetentionDays())}, nil
	})
}

// SiteAnalyticsOpensByHour maps the analytics module's Opens by hour report onto the DTO.
func (h *Handlers) SiteAnalyticsOpensByHour(ctx context.Context, params siteapi.SiteAnalyticsOpensByHourParams) (siteapi.SiteAnalyticsOpensByHourRes, error) {
	return analyticsReport(ctx, params.Range, func(s *ent.Scoped, w analytics.Window) (siteapi.SiteAnalyticsOpensByHourRes, error) {
		report, err := h.analytics.OpensByHour(ctx, s, w)
		if err != nil {
			return nil, err
		}
		cells := make([]siteapi.SiteAnalyticsOpensCell, len(report.Cells))
		for i, c := range report.Cells {
			cells[i] = siteapi.SiteAnalyticsOpensCell{Weekday: narrow.Int32(c.Weekday), Hour: narrow.Int32(c.Hour), Opens: narrow.Int32(c.Opens)}
		}
		return &siteapi.SiteAnalyticsOpensByHour{Cells: cells, Total: narrow.Int32(report.Total)}, nil
	})
}

// SiteAnalyticsNewPeople maps the New people report onto the DTO.
func (h *Handlers) SiteAnalyticsNewPeople(ctx context.Context, params siteapi.SiteAnalyticsNewPeopleParams) (siteapi.SiteAnalyticsNewPeopleRes, error) {
	return analyticsReport(ctx, params.Range, func(s *ent.Scoped, w analytics.Window) (siteapi.SiteAnalyticsNewPeopleRes, error) {
		report, err := h.analytics.NewPeople(ctx, s, w)
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
	})
}

// SiteAnalyticsRevenue maps the analytics module's Revenue onto the DTO (ADR 0064). The
// operation's default range is 30 days, declared in the contract.
func (h *Handlers) SiteAnalyticsRevenue(ctx context.Context, params siteapi.SiteAnalyticsRevenueParams) (siteapi.SiteAnalyticsRevenueRes, error) {
	s := workspaceScope(ctx)
	report, err := h.analytics.Revenue(ctx, s, revenueWindow(params.Range))
	if err != nil {
		return nil, err
	}
	r := mapper.RevenueToResource(report)
	return &r, nil
}

// revenueWindow maps the selectable range to the module's window. The range is always set: the
// contract gives it its default.
func revenueWindow(r siteapi.OptSiteAnalyticsRevenueRange) analytics.Window {
	switch r.Value {
	case siteapi.SiteAnalyticsRevenueRange7d:
		return analytics.Window7Days
	case siteapi.SiteAnalyticsRevenueRange90d:
		return analytics.Window90Days
	default:
		return analytics.Window30Days
	}
}
