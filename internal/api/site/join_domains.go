package site

import (
	"context"
	"net/http"

	"github.com/samber/lo"

	"github.com/getsphericon/sphericon/ent"
	siteapi "github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/accounts"
)

// SiteJoinDomainsList lists the Workspace's Join domains. Permission join_domains:manage.
func (h *Handlers) SiteJoinDomainsList(ctx context.Context, params siteapi.SiteJoinDomainsListParams) (siteapi.SiteJoinDomainsListRes, error) {
	s, role, err := h.scopedWithRoleFor(ctx, params.WorkspaceId)
	if err != nil {
		return nil, err
	}
	if err := accounts.Require(role, accounts.PermJoinDomainsManage); err != nil {
		return nil, err
	}

	rows, err := h.joinDomains.List(ctx, s)
	if err != nil {
		return nil, err
	}
	items := lo.Map(rows, func(d *ent.JoinDomain, _ int) siteapi.SiteJoinDomainResource { return mapper.JoinDomainToResource(d) })
	res := siteapi.SiteJoinDomainsListOKApplicationJSON(items)
	return &res, nil
}

// SiteJoinDomainsAdd adds a pending Join domain. Permission join_domains:manage.
func (h *Handlers) SiteJoinDomainsAdd(ctx context.Context, req *siteapi.SiteAddJoinDomainInput, params siteapi.SiteJoinDomainsAddParams) (siteapi.SiteJoinDomainsAddRes, error) {
	s, role, err := h.scopedWithRoleFor(ctx, params.WorkspaceId)
	if err != nil {
		return nil, err
	}
	if err := accounts.Require(role, accounts.PermJoinDomainsManage); err != nil {
		return nil, err
	}

	d, err := h.joinDomains.Add(ctx, s, req.Domain)
	if err != nil {
		return nil, err
	}
	res := mapper.JoinDomainToResource(d)
	return &res, nil
}

// SiteJoinDomainsVerify checks the TXT record live and verifies the domain. Permission join_domains:manage.
func (h *Handlers) SiteJoinDomainsVerify(ctx context.Context, params siteapi.SiteJoinDomainsVerifyParams) (siteapi.SiteJoinDomainsVerifyRes, error) {
	s, role, err := h.scopedWithRoleFor(ctx, params.WorkspaceId)
	if err != nil {
		return nil, err
	}
	if err := accounts.Require(role, accounts.PermJoinDomainsManage); err != nil {
		return nil, err
	}

	d, err := h.joinDomains.Verify(ctx, s, int64(params.ID))
	switch {
	case ent.IsNotFound(err):
		v := siteapi.SiteJoinDomainsVerifyNotFound(problem(http.StatusNotFound, "join domain not found"))
		return &v, nil
	case err != nil:
		return nil, err
	}
	res := mapper.JoinDomainToResource(d)
	return &res, nil
}

// SiteJoinDomainsRemove removes a Join domain claim. Permission join_domains:manage.
func (h *Handlers) SiteJoinDomainsRemove(ctx context.Context, params siteapi.SiteJoinDomainsRemoveParams) (siteapi.SiteJoinDomainsRemoveRes, error) {
	s, role, err := h.scopedWithRoleFor(ctx, params.WorkspaceId)
	if err != nil {
		return nil, err
	}
	if err := accounts.Require(role, accounts.PermJoinDomainsManage); err != nil {
		return nil, err
	}

	err = h.joinDomains.Remove(ctx, s, int64(params.ID))
	if ent.IsNotFound(err) {
		v := siteapi.SiteJoinDomainsRemoveNotFound(problem(http.StatusNotFound, "join domain not found"))
		return &v, nil
	}
	if err != nil {
		return nil, err
	}
	return &siteapi.SiteJoinDomainsRemoveNoContent{}, nil
}

// SiteDomainJoinGet reads the Workspace's domain join switch. Permission join_domains:manage.
func (h *Handlers) SiteDomainJoinGet(ctx context.Context, params siteapi.SiteDomainJoinGetParams) (siteapi.SiteDomainJoinGetRes, error) {
	s, role, err := h.scopedWithRoleFor(ctx, params.WorkspaceId)
	if err != nil {
		return nil, err
	}
	if err := accounts.Require(role, accounts.PermJoinDomainsManage); err != nil {
		return nil, err
	}

	enabled, err := h.accounts.DomainJoinEnabled(ctx, s)
	if err != nil {
		return nil, err
	}
	return &siteapi.SiteDomainJoinResource{Enabled: enabled}, nil
}

// SiteDomainJoinSet switches the Workspace's domain join on or off. Permission join_domains:manage.
func (h *Handlers) SiteDomainJoinSet(ctx context.Context, req *siteapi.SiteSetDomainJoinInput, params siteapi.SiteDomainJoinSetParams) (siteapi.SiteDomainJoinSetRes, error) {
	s, role, err := h.scopedWithRoleFor(ctx, params.WorkspaceId)
	if err != nil {
		return nil, err
	}
	if err := accounts.Require(role, accounts.PermJoinDomainsManage); err != nil {
		return nil, err
	}

	w, err := h.accounts.SetDomainJoin(ctx, s, h.actor(ctx), req.Enabled)
	if err != nil {
		return nil, err
	}
	return &siteapi.SiteDomainJoinResource{Enabled: w.DomainJoinEnabled}, nil
}
