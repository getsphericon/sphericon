package site

import (
	"context"

	siteapi "github.com/getsphericon/sphericon/gen/site"
)

// SiteJoinDomainsList lists the Workspace's Join domains. Permission join_domains:manage.
func (h *Handlers) SiteJoinDomainsList(ctx context.Context, _ siteapi.SiteJoinDomainsListParams) ([]siteapi.SiteJoinDomainResource, error) {
	s := workspaceScope(ctx)

	rows, err := h.joinDomains.List(ctx, s)
	if err != nil {
		return nil, err
	}
	items := mapper.JoinDomainsToResources(rows)
	return items, nil
}

// SiteJoinDomainsAdd adds a pending Join domain. Permission join_domains:manage.
func (h *Handlers) SiteJoinDomainsAdd(ctx context.Context, req *siteapi.SiteAddJoinDomainInput, _ siteapi.SiteJoinDomainsAddParams) (*siteapi.SiteJoinDomainResource, error) {
	s := workspaceScope(ctx)

	d, err := h.joinDomains.Add(ctx, s, req.Domain)
	if err != nil {
		return nil, err
	}
	res := mapper.JoinDomainToResource(d)
	return &res, nil
}

// SiteJoinDomainsVerify checks the TXT record live and verifies the domain. Permission join_domains:manage.
func (h *Handlers) SiteJoinDomainsVerify(ctx context.Context, params siteapi.SiteJoinDomainsVerifyParams) (*siteapi.SiteJoinDomainResource, error) {
	s := workspaceScope(ctx)

	d, err := h.joinDomains.Verify(ctx, s, int64(params.ID))
	if err != nil {
		return nil, err
	}
	res := mapper.JoinDomainToResource(d)
	return &res, nil
}

// SiteJoinDomainsRemove removes a Join domain claim. Permission join_domains:manage.
func (h *Handlers) SiteJoinDomainsRemove(ctx context.Context, params siteapi.SiteJoinDomainsRemoveParams) error {
	s := workspaceScope(ctx)

	err := h.joinDomains.Remove(ctx, s, int64(params.ID))
	if err != nil {
		return err
	}
	return nil
}

// SiteDomainJoinGet reads the Workspace's domain join switch. Permission join_domains:manage.
func (h *Handlers) SiteDomainJoinGet(ctx context.Context, _ siteapi.SiteDomainJoinGetParams) (*siteapi.SiteDomainJoinResource, error) {
	s := workspaceScope(ctx)

	enabled, err := h.accounts.DomainJoinEnabled(ctx, s)
	if err != nil {
		return nil, err
	}
	return &siteapi.SiteDomainJoinResource{Enabled: enabled}, nil
}

// SiteDomainJoinSet switches the Workspace's domain join on or off. Permission join_domains:manage.
func (h *Handlers) SiteDomainJoinSet(ctx context.Context, req *siteapi.SiteSetDomainJoinInput, _ siteapi.SiteDomainJoinSetParams) (*siteapi.SiteDomainJoinResource, error) {
	s := workspaceScope(ctx)

	w, err := h.accounts.SetDomainJoin(ctx, s, h.actor(ctx), req.Enabled)
	if err != nil {
		return nil, err
	}
	return mapper.DomainJoinToResource(w), nil
}
