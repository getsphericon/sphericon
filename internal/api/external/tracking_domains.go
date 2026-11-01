package external

import (
	"context"

	"github.com/getsphericon/sphericon/ent"
	externalapi "github.com/getsphericon/sphericon/gen/external"
	"github.com/getsphericon/sphericon/internal/api/auth"
)

// The Tracking domain operations are thin adapters over internal/trackingdomains (ADR 0071), the
// module /site shares. No operation can set a state: only the live check does.

// trackingDomainResource maps the row, then attaches the CNAME to publish, whose target the
// module knows and the mapper cannot see.
func (h *Handlers) trackingDomainResource(d *ent.TrackingDomain) externalapi.TrackingDomainResource {
	res := mapper.TrackingDomainToResource(d)
	res.Record = mapper.CnameRecordToResource(h.trackingDomains.Record(d))
	return res
}

// TrackingDomainsList returns the Workspace's Tracking domains with the CNAME to publish.
func (h *Handlers) TrackingDomainsList(ctx context.Context) (externalapi.TrackingDomainsListRes, error) {
	rows, err := h.trackingDomains.List(ctx, auth.TokenScoped(ctx))
	if err != nil {
		return nil, err
	}
	items := make([]externalapi.TrackingDomainResource, len(rows))
	for i, d := range rows {
		items[i] = h.trackingDomainResource(d)
	}
	res := externalapi.TrackingDomainsListOKApplicationJSON(items)
	return &res, nil
}

// TrackingDomainsSet sets the Workspace's Tracking domain.
func (h *Handlers) TrackingDomainsSet(ctx context.Context, req *externalapi.SetTrackingDomainInput) (externalapi.TrackingDomainsSetRes, error) {
	d, err := h.trackingDomains.Set(ctx, auth.TokenScoped(ctx), req.Host)
	if err != nil {
		return nil, err
	}
	res := h.trackingDomainResource(d)
	return &res, nil
}

// TrackingDomainsVerify checks a Tracking domain now.
func (h *Handlers) TrackingDomainsVerify(ctx context.Context, params externalapi.TrackingDomainsVerifyParams) (externalapi.TrackingDomainsVerifyRes, error) {
	d, err := h.trackingDomains.Verify(ctx, auth.TokenScoped(ctx), int64(params.ID))
	if err != nil {
		return nil, err
	}
	res := h.trackingDomainResource(d)
	return &res, nil
}

// TrackingDomainsRemove removes a Tracking domain by its role.
func (h *Handlers) TrackingDomainsRemove(ctx context.Context, params externalapi.TrackingDomainsRemoveParams) (externalapi.TrackingDomainsRemoveRes, error) {
	if err := h.trackingDomains.Remove(ctx, auth.TokenScoped(ctx), int64(params.ID)); err != nil {
		return nil, err
	}
	return &externalapi.TrackingDomainsRemoveNoContent{}, nil
}
