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

// joinDomainResource builds the API resource, computing the TXT record the Owner publishes.
func joinDomainResource(d *ent.JoinDomain) siteapi.SiteJoinDomainResource {
	rec := joindomains.RecordOf(d)
	res := siteapi.SiteJoinDomainResource{
		ID:        siteapi.EntityId(d.ID),
		Domain:    d.Domain,
		Verified:  joindomains.Verified(d),
		Record:    siteapi.SiteDnsRecord{Type: siteapi.SiteDnsRecordTypeTXT, Host: rec.Host, Value: rec.Value},
		CreatedAt: siteapi.Timestamp(d.CreatedAt),
		UpdatedAt: siteapi.Timestamp(d.UpdatedAt),
	}
	if d.LastCheckedAt != nil {
		res.LastCheckedAt = siteapi.NewOptNilTimestamp(siteapi.Timestamp(*d.LastCheckedAt))
	}
	if d.VerifiedAt != nil {
		res.VerifiedAt = siteapi.NewOptNilTimestamp(siteapi.Timestamp(*d.VerifiedAt))
	}
	return res
}

// SiteJoinDomainsList lists the Workspace's Join domains. Owners only.
func (h *Handlers) SiteJoinDomainsList(ctx context.Context, params siteapi.SiteJoinDomainsListParams) (siteapi.SiteJoinDomainsListRes, error) {
	s, role, err := h.scopedWithRoleFor(ctx, params.WorkspaceId)
	if ent.IsNotFound(err) {
		v := siteapi.SiteJoinDomainsListNotFound(problem(http.StatusNotFound, "workspace not found"))
		return &v, nil
	}
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
	items := lo.Map(rows, func(d *ent.JoinDomain, _ int) siteapi.SiteJoinDomainResource { return joinDomainResource(d) })
	res := siteapi.SiteJoinDomainsListOKApplicationJSON(items)
	return &res, nil
}

// SiteJoinDomainsAdd adds a pending Join domain. Owners only.
func (h *Handlers) SiteJoinDomainsAdd(ctx context.Context, req *siteapi.SiteAddJoinDomainInput, params siteapi.SiteJoinDomainsAddParams) (siteapi.SiteJoinDomainsAddRes, error) {
	s, role, err := h.scopedWithRoleFor(ctx, params.WorkspaceId)
	if ent.IsNotFound(err) {
		v := siteapi.SiteJoinDomainsAddNotFound(problem(http.StatusNotFound, "workspace not found"))
		return &v, nil
	}
	if err != nil {
		return nil, err
	}
	if !accounts.CanManageJoinDomains(role) {
		v := siteapi.SiteJoinDomainsAddForbidden(problem(http.StatusForbidden, i18n.T("errors.join_domain_forbidden", nil)))
		return &v, nil
	}

	d, err := h.joinDomains.Add(ctx, s, req.Domain)
	var verr *joindomains.ValidationError
	switch {
	case errors.As(err, &verr):
		v := siteapi.SiteJoinDomainsAddUnprocessableEntity(problemWithErrors(http.StatusUnprocessableEntity, i18n.T("errors.validation_failed", nil), map[string][]string{
			verr.Field: {verr.Message},
		}))
		return &v, nil
	case errors.Is(err, joindomains.ErrAlreadyExists):
		v := siteapi.SiteJoinDomainsAddConflict(problem(http.StatusConflict, i18n.T("errors.join_domain_exists", nil)))
		return &v, nil
	case err != nil:
		return nil, err
	}
	res := joinDomainResource(d)
	return &res, nil
}

// SiteJoinDomainsVerify checks the TXT record live and verifies the domain. Owners only.
func (h *Handlers) SiteJoinDomainsVerify(ctx context.Context, params siteapi.SiteJoinDomainsVerifyParams) (siteapi.SiteJoinDomainsVerifyRes, error) {
	s, role, err := h.scopedWithRoleFor(ctx, params.WorkspaceId)
	if ent.IsNotFound(err) {
		v := siteapi.SiteJoinDomainsVerifyNotFound(problem(http.StatusNotFound, "workspace not found"))
		return &v, nil
	}
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
	res := joinDomainResource(d)
	return &res, nil
}

// SiteJoinDomainsRemove removes a Join domain claim. Owners only.
func (h *Handlers) SiteJoinDomainsRemove(ctx context.Context, params siteapi.SiteJoinDomainsRemoveParams) (siteapi.SiteJoinDomainsRemoveRes, error) {
	s, role, err := h.scopedWithRoleFor(ctx, params.WorkspaceId)
	if ent.IsNotFound(err) {
		v := siteapi.SiteJoinDomainsRemoveNotFound(problem(http.StatusNotFound, "workspace not found"))
		return &v, nil
	}
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
