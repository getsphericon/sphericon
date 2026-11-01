package external

import (
	"context"

	"github.com/getsphericon/sphericon/ent"
	externalapi "github.com/getsphericon/sphericon/gen/external"
	"github.com/getsphericon/sphericon/internal/api/auth"
	"github.com/getsphericon/sphericon/internal/pagination"
)

// The Sending domain operations are thin adapters over internal/sendingdomains, the
// module /site shares: validation, DKIM keypair minting and sealing, and the verify
// trigger live there. The private key is never part of a response, and no operation
// can set `verified`: only the live DKIM check does.

// SendingDomainsList returns the Workspace's Sending domains with the DNS records to publish.
func (h *Handlers) SendingDomainsList(ctx context.Context, params externalapi.SendingDomainsListParams) (externalapi.SendingDomainsListRes, error) {
	page, err := h.sendingDomains.List(ctx, auth.TokenScoped(ctx), pagination.ParamsOf(params.Page, params.PageSize), params.Archived.Or(false))
	if err != nil {
		return nil, err
	}
	return mapper.SendingDomainsListPage(pagination.Map(page, h.sendingDomainResource)), nil
}

// SendingDomainsCreate adds a Sending domain; the DKIM keypair is minted and sealed.
func (h *Handlers) SendingDomainsCreate(ctx context.Context, req *externalapi.CreateSendingDomainInput) (externalapi.SendingDomainsCreateRes, error) {
	row, err := h.sendingDomains.Create(ctx, auth.TokenScoped(ctx), inputs.SendingDomainCreate(req))
	if err != nil {
		return nil, err
	}
	res := h.sendingDomainResource(row)
	return &res, nil
}

// SendingDomainsGet returns one Sending domain.
func (h *Handlers) SendingDomainsGet(ctx context.Context, params externalapi.SendingDomainsGetParams) (externalapi.SendingDomainsGetRes, error) {
	id := int64(params.ID)
	row, err := h.sendingDomains.Get(ctx, auth.TokenScoped(ctx), id)
	if err != nil {
		return nil, err
	}
	res := h.sendingDomainResource(row)
	return &res, nil
}

// SendingDomainsUpdate changes the DKIM selector, which unverifies the domain.
func (h *Handlers) SendingDomainsUpdate(ctx context.Context, req *externalapi.UpdateSendingDomainInput, params externalapi.SendingDomainsUpdateParams) (externalapi.SendingDomainsUpdateRes, error) {
	id := int64(params.ID)
	row, err := h.sendingDomains.Update(ctx, auth.TokenScoped(ctx), id, inputs.SendingDomainUpdate(req))
	if err != nil {
		return nil, err
	}
	res := h.sendingDomainResource(row)
	return &res, nil
}

// SendingDomainsArchive archives a Sending domain (ADR 0049): it leaves the list. Live sends from
// it refuse it with 409.
func (h *Handlers) SendingDomainsArchive(ctx context.Context, params externalapi.SendingDomainsArchiveParams) (externalapi.SendingDomainsArchiveRes, error) {
	id := int64(params.ID)
	s := auth.TokenScoped(ctx)
	row, err := h.sendingDomains.Archive(ctx, s, id)
	if err != nil {
		return nil, err
	}
	res := h.sendingDomainResource(row)
	return &res, nil
}

// SendingDomainsUnarchive restores an archived Sending domain and starts a DNS check.
func (h *Handlers) SendingDomainsUnarchive(ctx context.Context, params externalapi.SendingDomainsUnarchiveParams) (externalapi.SendingDomainsUnarchiveRes, error) {
	id := int64(params.ID)
	s := auth.TokenScoped(ctx)
	row, err := h.sendingDomains.Unarchive(ctx, s, id)
	if err != nil {
		return nil, err
	}
	res := h.sendingDomainResource(row)
	return &res, nil
}

// SendingDomainsVerify runs the live DNS check and returns the domain as the check left it
// (`verified`, `lastCheckedAt`).
func (h *Handlers) SendingDomainsVerify(ctx context.Context, params externalapi.SendingDomainsVerifyParams) (externalapi.SendingDomainsVerifyRes, error) {
	id := int64(params.ID)
	s := auth.TokenScoped(ctx)
	row, err := h.sendingDomains.Verify(ctx, s, id)
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
	return res
}
