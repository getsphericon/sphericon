package site

import (
	"context"
	"errors"
	"net/http"

	"github.com/samber/lo"

	"github.com/getsphericon/sphericon/ent"
	siteapi "github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/accounts"
	"github.com/getsphericon/sphericon/internal/narrow"
	"github.com/getsphericon/sphericon/internal/pagination"
	"github.com/getsphericon/sphericon/internal/sendingdomains"
)

// sendingDomainResource maps the row, then attaches the DNS records the user publishes, which
// the sendingdomains module derives and the mapper cannot see.
func (h *Handlers) sendingDomainResource(d *ent.SendingDomain) siteapi.SiteSendingDomainResource {
	rec := h.sendingDomains.Records(d)
	res := mapper.SendingDomainToResource(d)
	res.DkimRecord = mapper.DnsRecordToResource(rec.DKIM)
	res.SpfRecord = mapper.DnsRecordToResource(rec.SPF)
	res.DmarcRecord = mapper.DnsRecordToResource(rec.DMARC)
	if rec.Tracking != nil {
		res.TrackingRecord = siteapi.NewOptSiteTrackingRecord(mapper.TrackingRecordToResource(*rec.Tracking))
	}
	return res
}

func (h *Handlers) SiteSendingDomainsList(ctx context.Context, params siteapi.SiteSendingDomainsListParams) (siteapi.SiteSendingDomainsListRes, error) {
	s, err := h.scopedFor(ctx, params.WorkspaceId)
	if err != nil {
		return nil, err
	}

	page, err := h.sendingDomains.List(ctx, s, pagination.ParamsOf(params.Page, params.PageSize), params.Archived.Or(false))
	if err != nil {
		return nil, err
	}

	return &siteapi.SiteSendingDomainsListOK{
		Items: lo.Map(page.Items, func(d *ent.SendingDomain, _ int) siteapi.SiteSendingDomainResource {
			return h.sendingDomainResource(d)
		}),
		Page:       narrow.Int32(page.Page),
		PageSize:   narrow.Int32(page.PageSize),
		TotalItems: narrow.Int32(page.TotalItems),
		TotalPages: narrow.Int32(page.TotalPages),
	}, nil
}

func (h *Handlers) SiteSendingDomainsCreate(ctx context.Context, req *siteapi.SiteCreateSendingDomainInput, params siteapi.SiteSendingDomainsCreateParams) (siteapi.SiteSendingDomainsCreateRes, error) {
	s, role, err := h.scopedWithRoleFor(ctx, params.WorkspaceId)
	if err != nil {
		return nil, err
	}
	if err := accounts.Require(role, accounts.PermSendingDomainsManage); err != nil {
		return nil, err
	}

	d, err := h.sendingDomains.Create(ctx, s, sendingdomains.CreateInput{Domain: req.Domain, Selector: req.DkimSelector.Or("")})
	switch {
	case errors.Is(err, sendingdomains.ErrAlreadyExists):
		v := siteapi.SiteSendingDomainsCreateConflict(problem(http.StatusConflict, "this domain is already added"))
		return &v, nil
	case err != nil:
		return nil, err
	}
	res := h.sendingDomainResource(d)
	return &res, nil
}

func (h *Handlers) SiteSendingDomainsGet(ctx context.Context, params siteapi.SiteSendingDomainsGetParams) (siteapi.SiteSendingDomainsGetRes, error) {
	s, err := h.scopedFor(ctx, params.WorkspaceId)
	if err != nil {
		return nil, err
	}

	id := int64(params.ID)
	d, err := h.sendingDomains.Get(ctx, s, id)
	if ent.IsNotFound(err) {
		v := siteapi.SiteSendingDomainsGetNotFound(problem(http.StatusNotFound, "sending domain not found"))
		return &v, nil
	}
	if err != nil {
		return nil, err
	}
	res := h.sendingDomainResource(d)
	return &res, nil
}

// SiteSendingDomainsArchive archives a sending domain (ADR 0049): it leaves the list and serves
// no Tracking domain host. Live sends from it refuse it with 409.
func (h *Handlers) SiteSendingDomainsArchive(ctx context.Context, params siteapi.SiteSendingDomainsArchiveParams) (siteapi.SiteSendingDomainsArchiveRes, error) {
	s, role, err := h.scopedWithRoleFor(ctx, params.WorkspaceId)
	if err != nil {
		return nil, err
	}
	if err := accounts.Require(role, accounts.PermSendingDomainsManage); err != nil {
		return nil, err
	}

	id := int64(params.ID)
	err = h.sendingDomains.Archive(ctx, s, id)
	if ent.IsNotFound(err) {
		v := siteapi.SiteSendingDomainsArchiveNotFound(problem(http.StatusNotFound, "sending domain not found"))
		return &v, nil
	}
	if err != nil {
		return nil, err
	}
	d, err := h.sendingDomains.Get(ctx, s, id)
	if err != nil {
		return nil, err
	}
	res := h.sendingDomainResource(d)
	return &res, nil
}

// SiteSendingDomainsUnarchive restores an archived sending domain and starts a DNS check.
func (h *Handlers) SiteSendingDomainsUnarchive(ctx context.Context, params siteapi.SiteSendingDomainsUnarchiveParams) (siteapi.SiteSendingDomainsUnarchiveRes, error) {
	s, role, err := h.scopedWithRoleFor(ctx, params.WorkspaceId)
	if err != nil {
		return nil, err
	}
	if err := accounts.Require(role, accounts.PermSendingDomainsManage); err != nil {
		return nil, err
	}

	id := int64(params.ID)
	err = h.sendingDomains.Unarchive(ctx, s, id)
	switch {
	case errors.Is(err, sendingdomains.ErrAlreadyExists):
		v := siteapi.SiteSendingDomainsUnarchiveConflict(problem(http.StatusConflict, "this domain was added again; archive that one first"))
		return &v, nil
	case ent.IsNotFound(err):
		v := siteapi.SiteSendingDomainsUnarchiveNotFound(problem(http.StatusNotFound, "sending domain not found"))
		return &v, nil
	case err != nil:
		return nil, err
	}
	d, err := h.sendingDomains.Get(ctx, s, id)
	if err != nil {
		return nil, err
	}
	res := h.sendingDomainResource(d)
	return &res, nil
}

func (h *Handlers) SiteSendingDomainsVerify(ctx context.Context, params siteapi.SiteSendingDomainsVerifyParams) (siteapi.SiteSendingDomainsVerifyRes, error) {
	s, role, err := h.scopedWithRoleFor(ctx, params.WorkspaceId)
	if err != nil {
		return nil, err
	}
	if err := accounts.Require(role, accounts.PermSendingDomainsManage); err != nil {
		return nil, err
	}

	id := int64(params.ID)
	d, err := h.sendingDomains.Verify(ctx, s, id)
	if ent.IsNotFound(err) {
		v := siteapi.SiteSendingDomainsVerifyNotFound(problem(http.StatusNotFound, "sending domain not found"))
		return &v, nil
	}
	if err != nil {
		return nil, err
	}
	res := h.sendingDomainResource(d)
	return &res, nil
}
