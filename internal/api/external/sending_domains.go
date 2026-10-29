package external

import (
	"context"
	"errors"
	"net/http"

	"github.com/samber/lo"

	"github.com/getsphericon/sphericon/ent"
	externalapi "github.com/getsphericon/sphericon/gen/external"
	"github.com/getsphericon/sphericon/internal/api/auth"
	"github.com/getsphericon/sphericon/internal/convert"
	"github.com/getsphericon/sphericon/internal/narrow"
	"github.com/getsphericon/sphericon/internal/pagination"
	"github.com/getsphericon/sphericon/internal/sendingdomains"
)

// The Sending domain operations are thin adapters over internal/sendingdomains, the
// module /site shares: validation, DKIM keypair minting and sealing, and the verify
// trigger live there. The private key is never part of a response, and no operation
// can set `verified`: only the live DKIM check does.

// SendingDomainsList returns the Workspace's Sending domains with the DNS records to publish.
func (h *Handlers) SendingDomainsList(ctx context.Context, params externalapi.SendingDomainsListParams) (externalapi.SendingDomainsListRes, error) {
	if !auth.HasScope(auth.GetTokenAuth(ctx), externalapi.ApiTokenScopeSendingDomainsRead) {
		res := externalapi.SendingDomainsListUnauthorized(problem(http.StatusUnauthorized, "insufficient scope"))
		return &res, nil
	}
	page, err := h.sendingDomains.List(ctx, auth.TokenScoped(ctx), pagination.ParamsOf(params.Page, params.PageSize), params.Archived.Or(false))
	if err != nil {
		return nil, err
	}
	return &externalapi.SendingDomainsListOK{
		Items: lo.Map(page.Items, func(d *ent.SendingDomain, _ int) externalapi.SendingDomainResource {
			return h.sendingDomainResource(d)
		}),
		Page:       narrow.Int32(page.Page),
		PageSize:   narrow.Int32(page.PageSize),
		TotalItems: narrow.Int32(page.TotalItems),
		TotalPages: narrow.Int32(page.TotalPages),
	}, nil
}

// SendingDomainsCreate adds a Sending domain; the DKIM keypair is minted and sealed.
func (h *Handlers) SendingDomainsCreate(ctx context.Context, req *externalapi.CreateSendingDomainInput) (externalapi.SendingDomainsCreateRes, error) {
	if !auth.HasScope(auth.GetTokenAuth(ctx), externalapi.ApiTokenScopeSendingDomainsWrite) {
		res := externalapi.SendingDomainsCreateUnauthorized(problem(http.StatusUnauthorized, "insufficient scope"))
		return &res, nil
	}
	row, err := h.sendingDomains.Create(ctx, auth.TokenScoped(ctx), sendingdomains.CreateInput{
		Domain: req.Domain, Selector: req.DkimSelector.Or(""),
	})
	switch {
	case errors.Is(err, sendingdomains.ErrAlreadyExists):
		res := externalapi.SendingDomainsCreateConflict(problem(http.StatusConflict, err.Error()))
		return &res, nil
	case err != nil:
		return nil, err
	}
	res := h.sendingDomainResource(row)
	return &res, nil
}

// SendingDomainsGet returns one Sending domain.
func (h *Handlers) SendingDomainsGet(ctx context.Context, params externalapi.SendingDomainsGetParams) (externalapi.SendingDomainsGetRes, error) {
	if !auth.HasScope(auth.GetTokenAuth(ctx), externalapi.ApiTokenScopeSendingDomainsRead) {
		res := externalapi.SendingDomainsGetUnauthorized(problem(http.StatusUnauthorized, "insufficient scope"))
		return &res, nil
	}
	id := int64(params.ID)
	row, err := auth.TokenScoped(ctx).SendingDomain().Get(ctx, id)
	if ent.IsNotFound(err) {
		res := externalapi.SendingDomainsGetNotFound(problem(http.StatusNotFound, "sending domain not found"))
		return &res, nil
	}
	if err != nil {
		return nil, err
	}
	res := h.sendingDomainResource(row)
	return &res, nil
}

// SendingDomainsUpdate changes the DKIM selector, which unverifies the domain.
func (h *Handlers) SendingDomainsUpdate(ctx context.Context, req *externalapi.UpdateSendingDomainInput, params externalapi.SendingDomainsUpdateParams) (externalapi.SendingDomainsUpdateRes, error) {
	if !auth.HasScope(auth.GetTokenAuth(ctx), externalapi.ApiTokenScopeSendingDomainsWrite) {
		res := externalapi.SendingDomainsUpdateUnauthorized(problem(http.StatusUnauthorized, "insufficient scope"))
		return &res, nil
	}
	id := int64(params.ID)
	row, err := h.sendingDomains.Update(ctx, auth.TokenScoped(ctx), id, sendingdomains.UpdateInput{
		Selector: convert.Ptr(req.DkimSelector),
	})
	switch {
	case ent.IsNotFound(err):
		res := externalapi.SendingDomainsUpdateNotFound(problem(http.StatusNotFound, "sending domain not found"))
		return &res, nil
	case err != nil:
		return nil, err
	}
	res := h.sendingDomainResource(row)
	return &res, nil
}

// SendingDomainsArchive archives a Sending domain (ADR 0049): it leaves the list and serves no
// Tracking domain host. Live sends from it refuse it with 409.
func (h *Handlers) SendingDomainsArchive(ctx context.Context, params externalapi.SendingDomainsArchiveParams) (externalapi.SendingDomainsArchiveRes, error) {
	if !auth.HasScope(auth.GetTokenAuth(ctx), externalapi.ApiTokenScopeSendingDomainsWrite) {
		res := externalapi.SendingDomainsArchiveUnauthorized(problem(http.StatusUnauthorized, "insufficient scope"))
		return &res, nil
	}
	id := int64(params.ID)
	s := auth.TokenScoped(ctx)
	err := h.sendingDomains.Archive(ctx, s, id)
	if ent.IsNotFound(err) {
		res := externalapi.SendingDomainsArchiveNotFound(problem(http.StatusNotFound, "sending domain not found"))
		return &res, nil
	}
	if err != nil {
		return nil, err
	}
	row, err := s.SendingDomain().Get(ctx, id)
	if err != nil {
		return nil, err
	}
	res := h.sendingDomainResource(row)
	return &res, nil
}

// SendingDomainsUnarchive restores an archived Sending domain and starts a DNS check.
func (h *Handlers) SendingDomainsUnarchive(ctx context.Context, params externalapi.SendingDomainsUnarchiveParams) (externalapi.SendingDomainsUnarchiveRes, error) {
	if !auth.HasScope(auth.GetTokenAuth(ctx), externalapi.ApiTokenScopeSendingDomainsWrite) {
		res := externalapi.SendingDomainsUnarchiveUnauthorized(problem(http.StatusUnauthorized, "insufficient scope"))
		return &res, nil
	}
	id := int64(params.ID)
	s := auth.TokenScoped(ctx)
	err := h.sendingDomains.Unarchive(ctx, s, id)
	switch {
	case errors.Is(err, sendingdomains.ErrAlreadyExists):
		res := externalapi.SendingDomainsUnarchiveConflict(problem(http.StatusConflict, "this domain was added again; archive that one first"))
		return &res, nil
	case ent.IsNotFound(err):
		res := externalapi.SendingDomainsUnarchiveNotFound(problem(http.StatusNotFound, "sending domain not found"))
		return &res, nil
	case err != nil:
		return nil, err
	}
	row, err := s.SendingDomain().Get(ctx, id)
	if err != nil {
		return nil, err
	}
	res := h.sendingDomainResource(row)
	return &res, nil
}

// SendingDomainsVerify runs the live DNS check and returns the domain as the check left it
// (`verified`, `lastCheckedAt`).
func (h *Handlers) SendingDomainsVerify(ctx context.Context, params externalapi.SendingDomainsVerifyParams) (externalapi.SendingDomainsVerifyRes, error) {
	if !auth.HasScope(auth.GetTokenAuth(ctx), externalapi.ApiTokenScopeSendingDomainsWrite) {
		res := externalapi.SendingDomainsVerifyUnauthorized(problem(http.StatusUnauthorized, "insufficient scope"))
		return &res, nil
	}
	id := int64(params.ID)
	s := auth.TokenScoped(ctx)
	row, err := h.sendingDomains.Verify(ctx, s, id)
	if ent.IsNotFound(err) {
		res := externalapi.SendingDomainsVerifyNotFound(problem(http.StatusNotFound, "sending domain not found"))
		return &res, nil
	}
	if err != nil {
		return nil, err
	}
	res := h.sendingDomainResource(row)
	return &res, nil
}

// sendingDomainResource maps the row, then attaches the DNS records the user publishes, which
// the sendingdomains module derives and the mapper cannot see.
func (h *Handlers) sendingDomainResource(d *ent.SendingDomain) externalapi.SendingDomainResource {
	rec := h.sendingDomains.Records(d)
	res := mapper.SendingDomainToResource(d)
	res.DkimRecord = mapper.DnsRecordToResource(rec.DKIM)
	res.SpfRecord = mapper.DnsRecordToResource(rec.SPF)
	res.DmarcRecord = mapper.DnsRecordToResource(rec.DMARC)
	if rec.Tracking != nil {
		res.TrackingRecord = externalapi.NewOptTrackingRecord(mapper.TrackingRecordToResource(*rec.Tracking))
	}
	return res
}
