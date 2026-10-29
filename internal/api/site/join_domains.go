package site

import (
	"context"
	"errors"
	"net/http"

	"github.com/samber/lo"

	"github.com/getsphericon/sphericon/ent"
	siteapi "github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/accounts"
	"github.com/getsphericon/sphericon/internal/i18n"
	"github.com/getsphericon/sphericon/internal/joindomains"
)

// SiteJoinDomainsList lists the Workspace's Join domains. Owners only.
func (h *Handlers) SiteJoinDomainsList(ctx context.Context, params siteapi.SiteJoinDomainsListParams) (siteapi.SiteJoinDomainsListRes, error) {
	s, role, err := h.scopedWithRoleFor(ctx, params.WorkspaceId)
	if err != nil {
		return nil, err
	}
	if !accounts.CanManageJoinDomains(role) {
		v := siteapi.SiteJoinDomainsListForbidden(problem(http.StatusForbidden, i18n.T("errors.join_domain_forbidden", nil)))
		return &v, nil
	}

	rows, err := h.joinDomains.List(ctx, s)
	if err != nil {
		return nil, err
	}
	items := lo.Map(rows, func(d *ent.JoinDomain, _ int) siteapi.SiteJoinDomainResource { return mapper.JoinDomainToResource(d) })
	res := siteapi.SiteJoinDomainsListOKApplicationJSON(items)
	return &res, nil
}

// SiteJoinDomainsAdd adds a pending Join domain. Owners only.
func (h *Handlers) SiteJoinDomainsAdd(ctx context.Context, req *siteapi.SiteAddJoinDomainInput, params siteapi.SiteJoinDomainsAddParams) (siteapi.SiteJoinDomainsAddRes, error) {
	s, role, err := h.scopedWithRoleFor(ctx, params.WorkspaceId)
	if err != nil {
		return nil, err
	}
	if !accounts.CanManageJoinDomains(role) {
		v := siteapi.SiteJoinDomainsAddForbidden(problem(http.StatusForbidden, i18n.T("errors.join_domain_forbidden", nil)))
		return &v, nil
	}

	d, err := h.joinDomains.Add(ctx, s, req.Domain)
	switch {
	case errors.Is(err, joindomains.ErrAlreadyExists):
		v := siteapi.SiteJoinDomainsAddConflict(problem(http.StatusConflict, i18n.T("errors.join_domain_exists", nil)))
		return &v, nil
	case err != nil:
		return nil, err
	}
	res := mapper.JoinDomainToResource(d)
	return &res, nil
}

// SiteJoinDomainsVerify checks the TXT record live and verifies the domain. Owners only.
func (h *Handlers) SiteJoinDomainsVerify(ctx context.Context, params siteapi.SiteJoinDomainsVerifyParams) (siteapi.SiteJoinDomainsVerifyRes, error) {
	s, role, err := h.scopedWithRoleFor(ctx, params.WorkspaceId)
	if err != nil {
		return nil, err
	}
	if !accounts.CanManageJoinDomains(role) {
		v := siteapi.SiteJoinDomainsVerifyForbidden(problem(http.StatusForbidden, i18n.T("errors.join_domain_forbidden", nil)))
		return &v, nil
	}

	d, err := h.joinDomains.Verify(ctx, s, int64(params.ID))
	switch {
	case ent.IsNotFound(err):
		v := siteapi.SiteJoinDomainsVerifyNotFound(problem(http.StatusNotFound, "join domain not found"))
		return &v, nil
	case errors.Is(err, joindomains.ErrRecordNotFound):
		v := siteapi.SiteJoinDomainsVerifyUnprocessableEntity(problem(http.StatusUnprocessableEntity, i18n.T("errors.join_domain_record_missing", nil)))
		return &v, nil
	case errors.Is(err, joindomains.ErrHeldElsewhere):
		v := siteapi.SiteJoinDomainsVerifyConflict(problem(http.StatusConflict, i18n.T("errors.join_domain_held", nil)))
		return &v, nil
	case err != nil:
		return nil, err
	}
	res := mapper.JoinDomainToResource(d)
	return &res, nil
}

// SiteJoinDomainsRemove removes a Join domain claim. Owners only.
func (h *Handlers) SiteJoinDomainsRemove(ctx context.Context, params siteapi.SiteJoinDomainsRemoveParams) (siteapi.SiteJoinDomainsRemoveRes, error) {
	s, role, err := h.scopedWithRoleFor(ctx, params.WorkspaceId)
	if err != nil {
		return nil, err
	}
	if !accounts.CanManageJoinDomains(role) {
		v := siteapi.SiteJoinDomainsRemoveForbidden(problem(http.StatusForbidden, i18n.T("errors.join_domain_forbidden", nil)))
		return &v, nil
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

// SiteDomainJoinGet reads the Workspace's domain join switch. Owners only.
func (h *Handlers) SiteDomainJoinGet(ctx context.Context, params siteapi.SiteDomainJoinGetParams) (siteapi.SiteDomainJoinGetRes, error) {
	s, role, err := h.scopedWithRoleFor(ctx, params.WorkspaceId)
	if err != nil {
		return nil, err
	}
	if !accounts.CanManageJoinDomains(role) {
		v := siteapi.SiteDomainJoinGetForbidden(problem(http.StatusForbidden, i18n.T("errors.join_domain_forbidden", nil)))
		return &v, nil
	}

	enabled, err := h.accounts.DomainJoinEnabled(ctx, s)
	if err != nil {
		return nil, err
	}
	return &siteapi.SiteDomainJoinResource{Enabled: enabled}, nil
}

// SiteDomainJoinSet switches the Workspace's domain join on or off. Owners only.
func (h *Handlers) SiteDomainJoinSet(ctx context.Context, req *siteapi.SiteSetDomainJoinInput, params siteapi.SiteDomainJoinSetParams) (siteapi.SiteDomainJoinSetRes, error) {
	s, role, err := h.scopedWithRoleFor(ctx, params.WorkspaceId)
	if err != nil {
		return nil, err
	}
	if !accounts.CanManageJoinDomains(role) {
		v := siteapi.SiteDomainJoinSetForbidden(problem(http.StatusForbidden, i18n.T("errors.join_domain_forbidden", nil)))
		return &v, nil
	}

	w, err := h.accounts.SetDomainJoin(ctx, s, h.actor(ctx), req.Enabled)
	if err != nil {
		return nil, err
	}
	return &siteapi.SiteDomainJoinResource{Enabled: w.DomainJoinEnabled}, nil
}
