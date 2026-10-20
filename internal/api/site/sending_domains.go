package site

import (
	"context"
	"errors"
	"net/http"

	"github.com/samber/lo"

	"github.com/getsphericon/sphericon/ent"
	siteapi "github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/i18n"
	"github.com/getsphericon/sphericon/internal/inuse"
	"github.com/getsphericon/sphericon/internal/pagination"
	"github.com/getsphericon/sphericon/internal/sendingdomains"
)

// sendingDomainResource builds the API resource, computing the DNS records the
// user publishes. Built by hand (not goverter) because the records are derived,
// and the private key is never exposed.
func (h *Handlers) sendingDomainResource(d *ent.SendingDomain) siteapi.SiteSendingDomainResource {
	rec := h.sendingDomains.Records(d)

	res := siteapi.SiteSendingDomainResource{
		ID:             siteapi.EntityId(d.ID),
		Domain:         d.Domain,
		DkimSelector:   d.DkimSelector,
		Verified:       d.Verified,
		DkimRecord:     dnsRecord(rec.DKIM),
		SpfRecord:      dnsRecord(rec.SPF),
		DmarcRecord:    dnsRecord(rec.DMARC),
		TrackingRecord: trackingRecord(rec.Tracking),
		CreatedAt:      siteapi.Timestamp(d.CreatedAt),
		UpdatedAt:      siteapi.Timestamp(d.UpdatedAt),
	}
	if d.LastCheckedAt != nil {
		res.LastCheckedAt = siteapi.NewOptNilTimestamp(siteapi.Timestamp(*d.LastCheckedAt))
	}
	if d.VerifiedAt != nil {
		res.VerifiedAt = siteapi.NewOptNilTimestamp(siteapi.Timestamp(*d.VerifiedAt))
	}
	return res
}

func dnsRecord(r sendingdomains.Record) siteapi.SiteDnsRecord {
	return siteapi.SiteDnsRecord{Type: siteapi.SiteDnsRecordTypeTXT, Host: r.Host, Value: r.Value}
}

func (h *Handlers) SiteSendingDomainsList(ctx context.Context, params siteapi.SiteSendingDomainsListParams) (siteapi.SiteSendingDomainsListRes, error) {
	s, err := h.scopedFor(ctx, params.WorkspaceId)
	if ent.IsNotFound(err) {
		v := siteapi.SiteSendingDomainsListNotFound(problem(http.StatusNotFound, "workspace not found"))
		return &v, nil
	}
	if err != nil {
		return nil, err
	}

	page, err := h.sendingDomains.List(ctx, s, pagination.ParamsOf(params.Page, params.PageSize))
	if err != nil {
		return nil, err
	}

	return &siteapi.SiteSendingDomainsListOK{
		Items: lo.Map(page.Items, func(d *ent.SendingDomain, _ int) siteapi.SiteSendingDomainResource {
			return h.sendingDomainResource(d)
		}),
		Page:       int32(page.Page),
		PageSize:   int32(page.PageSize),
		TotalItems: int32(page.TotalItems),
		TotalPages: int32(page.TotalPages),
	}, nil
}

func (h *Handlers) SiteSendingDomainsCreate(ctx context.Context, req *siteapi.SiteCreateSendingDomainInput, params siteapi.SiteSendingDomainsCreateParams) (siteapi.SiteSendingDomainsCreateRes, error) {
	s, err := h.scopedFor(ctx, params.WorkspaceId)
	if ent.IsNotFound(err) {
		v := siteapi.SiteSendingDomainsCreateNotFound(problem(http.StatusNotFound, "workspace not found"))
		return &v, nil
	}
	if err != nil {
		return nil, err
	}

	d, err := h.sendingDomains.Create(ctx, s, sendingdomains.CreateInput{Domain: req.Domain, Selector: req.DkimSelector.Or("")})
	var verr *sendingdomains.ValidationError
	switch {
	case errors.As(err, &verr):
		v := siteapi.SiteSendingDomainsCreateUnprocessableEntity(problemWithErrors(http.StatusUnprocessableEntity, i18n.T("errors.validation_failed", nil), map[string][]string{
			verr.Field: {verr.Message},
		}))
		return &v, nil
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
	if ent.IsNotFound(err) {
		v := siteapi.SiteSendingDomainsGetNotFound(problem(http.StatusNotFound, "workspace not found"))
		return &v, nil
	}
	if err != nil {
		return nil, err
	}

	id := int64(params.ID)
	d, err := s.SendingDomain().Get(ctx, id)
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

func (h *Handlers) SiteSendingDomainsDelete(ctx context.Context, params siteapi.SiteSendingDomainsDeleteParams) (siteapi.SiteSendingDomainsDeleteRes, error) {
	s, err := h.scopedFor(ctx, params.WorkspaceId)
	if ent.IsNotFound(err) {
		v := siteapi.SiteSendingDomainsDeleteNotFound(problem(http.StatusNotFound, "workspace not found"))
		return &v, nil
	}
	if err != nil {
		return nil, err
	}

	id := int64(params.ID)
	err = h.sendingDomains.Delete(ctx, s, id)
	if refused, ok := inuse.As(err); ok {
		v := inUseProblem(refused)
		return &v, nil
	}
	if ent.IsNotFound(err) {
		v := siteapi.SiteSendingDomainsDeleteNotFound(problem(http.StatusNotFound, "sending domain not found"))
		return &v, nil
	}
	if err != nil {
		return nil, err
	}
	return &siteapi.SiteSendingDomainsDeleteNoContent{}, nil
}

func (h *Handlers) SiteSendingDomainsVerify(ctx context.Context, params siteapi.SiteSendingDomainsVerifyParams) (siteapi.SiteSendingDomainsVerifyRes, error) {
	s, err := h.scopedFor(ctx, params.WorkspaceId)
	if ent.IsNotFound(err) {
		v := siteapi.SiteSendingDomainsVerifyNotFound(problem(http.StatusNotFound, "workspace not found"))
		return &v, nil
	}
	if err != nil {
		return nil, err
	}

	id := int64(params.ID)
	err = h.sendingDomains.Verify(ctx, s, id)
	if ent.IsNotFound(err) {
		v := siteapi.SiteSendingDomainsVerifyNotFound(problem(http.StatusNotFound, "sending domain not found"))
		return &v, nil
	}
	if err != nil {
		return nil, err
	}
	return &siteapi.SiteSendingDomainsVerifyNoContent{}, nil
}

func trackingRecord(t *sendingdomains.TrackingRecord) siteapi.OptSiteTrackingRecord {
	if t == nil {
		return siteapi.OptSiteTrackingRecord{}
	}
	rec := siteapi.SiteTrackingRecord{
		Type:   siteapi.SiteTrackingRecordTypeCNAME,
		Host:   t.Host,
		Target: t.Target,
		State:  siteapi.SiteTrackingState(t.State),
	}
	if t.Reason != nil {
		rec.Reason = siteapi.NewOptSiteTrackingReason(siteapi.SiteTrackingReason(*t.Reason))
	}
	if t.CheckedAt != nil {
		rec.LastCheckedAt = siteapi.NewOptNilTimestamp(siteapi.Timestamp(*t.CheckedAt))
	}
	if t.StateChangedAt != nil {
		rec.StateChangedAt = siteapi.NewOptNilTimestamp(siteapi.Timestamp(*t.StateChangedAt))
	}
	return siteapi.NewOptSiteTrackingRecord(rec)
}
