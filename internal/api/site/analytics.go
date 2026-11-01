package site

import (
	"context"

	"github.com/getsphericon/sphericon/ent"
	siteapi "github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/analytics"
	"github.com/getsphericon/sphericon/internal/narrow"
)

// analyticsReport is the shell every report handler shares: it reads the selected window and hands
// it, with the scope of the Workspace of the path, to build, which maps the module's report onto
// the operation's response.
func analyticsReport[Res any](ctx context.Context, rng siteapi.OptSiteAnalyticsRange, build func(*ent.Scoped, analytics.Window) (Res, error)) (Res, error) {
	// 7d is the window of a request that names none.
	return build(workspaceScope(ctx), inputs.AnalyticsWindow(rng.Or(siteapi.SiteAnalyticsRange7d)))
}

// SiteAnalyticsOverview maps the analytics module's Overview onto the DTO.
func (h *Handlers) SiteAnalyticsOverview(ctx context.Context, params siteapi.SiteAnalyticsOverviewParams) (siteapi.SiteAnalyticsOverviewRes, error) {
	return analyticsReport(ctx, params.Range, func(s *ent.Scoped, w analytics.Window) (siteapi.SiteAnalyticsOverviewRes, error) {
		ov, err := h.analytics.Overview(ctx, s, w)
		if err != nil {
			return nil, err
		}
		return mapper.AnalyticsOverviewToResource(ov), nil
	})
}

// SiteAnalyticsEvents maps the analytics module's Events report onto the DTO.
func (h *Handlers) SiteAnalyticsEvents(ctx context.Context, params siteapi.SiteAnalyticsEventsParams) (siteapi.SiteAnalyticsEventsRes, error) {
	return analyticsReport(ctx, params.Range, func(s *ent.Scoped, w analytics.Window) (siteapi.SiteAnalyticsEventsRes, error) {
		rows, err := h.analytics.Events(ctx, s, w)
		if err != nil {
			return nil, err
		}
		return &siteapi.SiteAnalyticsEvents{Actions: mapper.AnalyticsEventActionsToResources(rows)}, nil
	})
}

// SiteAnalyticsSubscribedPeople maps the Subscribed people report onto the DTO.
func (h *Handlers) SiteAnalyticsSubscribedPeople(ctx context.Context, params siteapi.SiteAnalyticsSubscribedPeopleParams) (siteapi.SiteAnalyticsSubscribedPeopleRes, error) {
	return analyticsReport(ctx, params.Range, func(s *ent.Scoped, w analytics.Window) (siteapi.SiteAnalyticsSubscribedPeopleRes, error) {
		report, err := h.analytics.SubscribedPeople(ctx, s, w)
		if err != nil {
			return nil, err
		}
		return mapper.AnalyticsSubscribedPeopleToResource(report), nil
	})
}

// SiteAnalyticsClicksByLink maps the analytics module's Clicks by link report onto the DTO, with
// the Event retention the counts cover.
func (h *Handlers) SiteAnalyticsClicksByLink(ctx context.Context, params siteapi.SiteAnalyticsClicksByLinkParams) (siteapi.SiteAnalyticsClicksByLinkRes, error) {
	return analyticsReport(ctx, params.Range, func(s *ent.Scoped, w analytics.Window) (siteapi.SiteAnalyticsClicksByLinkRes, error) {
		links, err := h.analytics.ClicksByLink(ctx, s, w)
		if err != nil {
			return nil, err
		}
		return &siteapi.SiteAnalyticsClicksByLink{
			Links:               mapper.AnalyticsLinksToResources(links),
			EventsRetentionDays: narrow.Int32(h.analytics.RetentionDays()),
		}, nil
	})
}

// SiteAnalyticsOpensByHour maps the analytics module's Opens by hour report onto the DTO.
func (h *Handlers) SiteAnalyticsOpensByHour(ctx context.Context, params siteapi.SiteAnalyticsOpensByHourParams) (siteapi.SiteAnalyticsOpensByHourRes, error) {
	return analyticsReport(ctx, params.Range, func(s *ent.Scoped, w analytics.Window) (siteapi.SiteAnalyticsOpensByHourRes, error) {
		report, err := h.analytics.OpensByHour(ctx, s, w)
		if err != nil {
			return nil, err
		}
		return mapper.AnalyticsOpensByHourToResource(report), nil
	})
}

// SiteAnalyticsNewPeople maps the New people report onto the DTO.
func (h *Handlers) SiteAnalyticsNewPeople(ctx context.Context, params siteapi.SiteAnalyticsNewPeopleParams) (siteapi.SiteAnalyticsNewPeopleRes, error) {
	return analyticsReport(ctx, params.Range, func(s *ent.Scoped, w analytics.Window) (siteapi.SiteAnalyticsNewPeopleRes, error) {
		report, err := h.analytics.NewPeople(ctx, s, w)
		if err != nil {
			return nil, err
		}
		return mapper.AnalyticsNewPeopleToResource(report), nil
	})
}

// SiteAnalyticsRevenue maps the Workspace's revenue onto the DTO (ADR 0064). The
// operation's default range is 30 days, declared in the contract.
func (h *Handlers) SiteAnalyticsRevenue(ctx context.Context, params siteapi.SiteAnalyticsRevenueParams) (siteapi.SiteAnalyticsRevenueRes, error) {
	s := workspaceScope(ctx)
	report, err := h.analytics.Revenue(ctx, s, inputs.AnalyticsRevenueWindow(params.Range.Or(siteapi.SiteAnalyticsRevenueRange30d)))
	if err != nil {
		return nil, err
	}
	r := mapper.RevenueToResource(report)
	return &r, nil
}
