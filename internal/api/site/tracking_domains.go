package site

import (
	"context"

	"github.com/getsphericon/sphericon/ent"
	siteapi "github.com/getsphericon/sphericon/gen/site"
)

// The Tracking domain operations are thin adapters over internal/trackingdomains (ADR 0071), the
// module /api shares.

// trackingDomainResource maps the row, then attaches the CNAME to publish, whose target the
// module knows and the mapper cannot see.
func (h *Handlers) trackingDomainResource(d *ent.TrackingDomain) siteapi.SiteTrackingDomainResource {
	res := mapper.TrackingDomainToResource(d)
	res.Record = mapper.CnameRecordToResource(h.trackingDomains.Record(d))
	return res
}

// SiteTrackingDomainsList lists the Workspace's Tracking domains. Any member.
func (h *Handlers) SiteTrackingDomainsList(ctx context.Context, _ siteapi.SiteTrackingDomainsListParams) (siteapi.SiteTrackingDomainsListRes, error) {
	rows, err := h.trackingDomains.List(ctx, workspaceScope(ctx))
	if err != nil {
		return nil, err
	}
	items := make([]siteapi.SiteTrackingDomainResource, len(rows))
	for i, d := range rows {
		items[i] = h.trackingDomainResource(d)
	}
	res := siteapi.SiteTrackingDomainsListOKApplicationJSON(items)
	return &res, nil
}

// SiteTrackingDomainsSet sets the Workspace's Tracking domain. Permission tracking_domains:manage.
func (h *Handlers) SiteTrackingDomainsSet(ctx context.Context, req *siteapi.SetTrackingDomainInput, _ siteapi.SiteTrackingDomainsSetParams) (siteapi.SiteTrackingDomainsSetRes, error) {
	d, err := h.trackingDomains.Set(ctx, workspaceScope(ctx), req.Host)
	if err != nil {
		return nil, err
	}
	res := h.trackingDomainResource(d)
	return &res, nil
}

// SiteTrackingDomainsVerify checks a Tracking domain now. Permission tracking_domains:manage.
func (h *Handlers) SiteTrackingDomainsVerify(ctx context.Context, params siteapi.SiteTrackingDomainsVerifyParams) (siteapi.SiteTrackingDomainsVerifyRes, error) {
	d, err := h.trackingDomains.Verify(ctx, workspaceScope(ctx), int64(params.ID))
	if err != nil {
		return nil, err
	}
	res := h.trackingDomainResource(d)
	return &res, nil
}

// SiteTrackingDomainsRemove removes a Tracking domain by its role. Permission tracking_domains:manage.
func (h *Handlers) SiteTrackingDomainsRemove(ctx context.Context, params siteapi.SiteTrackingDomainsRemoveParams) (siteapi.SiteTrackingDomainsRemoveRes, error) {
	if err := h.trackingDomains.Remove(ctx, workspaceScope(ctx), int64(params.ID)); err != nil {
		return nil, err
	}
	return &siteapi.SiteTrackingDomainsRemoveNoContent{}, nil
}
