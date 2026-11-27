package external

import (
	"context"

	externalapi "github.com/getsphericon/sphericon/gen/external"
	"github.com/getsphericon/sphericon/internal/api/auth"
	"github.com/getsphericon/sphericon/internal/domain/sending/sendingdomains"
	"github.com/getsphericon/sphericon/internal/kernel/pagination"
)

// The Sending domain operations are thin adapters over internal/domain/sending/sendingdomains, the
// module /site shares: validation, DKIM keypair minting and sealing, and the verify
// trigger live there. The private key is never part of a response, and no operation
// can set `verified`: only the live DKIM check does.

// SendingDomainsList returns the Workspace's Sending domains with the DNS records to publish.
func (h *Handlers) SendingDomainsList(ctx context.Context, params externalapi.SendingDomainsListParams) (*externalapi.SendingDomainsListOK, error) {
	s := auth.TokenScoped(ctx)
	page, err := h.sendingDomains.List(ctx, s, pagination.ParamsOf(params.Page, params.PageSize))
	if err != nil {
		return nil, err
	}
	return mapper.SendingDomainsListPage(page.Map(h.sendingDomainResource)), nil
}

// SendingDomainsCreate adds a Sending domain; the DKIM keypair is minted and sealed.
func (h *Handlers) SendingDomainsCreate(ctx context.Context, req *externalapi.CreateSendingDomainInput) (*externalapi.SendingDomainResource, error) {
	s := auth.TokenScoped(ctx)
	row, err := h.sendingDomains.Create(ctx, s, inputs.SendingDomainCreate(req))
	if err != nil {
		return nil, err
	}
	res := h.sendingDomainResource(row)
	return &res, nil
}

// SendingDomainsGet returns one Sending domain.
func (h *Handlers) SendingDomainsGet(ctx context.Context, params externalapi.SendingDomainsGetParams) (*externalapi.SendingDomainResource, error) {
	s := auth.TokenScoped(ctx)
	id := int64(params.ID)
	row, err := h.sendingDomains.Get(ctx, s, id)
	if err != nil {
		return nil, err
	}
	res := h.sendingDomainResource(row)
	return &res, nil
}

// SendingDomainsUpdate changes the DKIM selector, which unverifies the domain.
func (h *Handlers) SendingDomainsUpdate(ctx context.Context, req *externalapi.UpdateSendingDomainInput, params externalapi.SendingDomainsUpdateParams) (*externalapi.SendingDomainResource, error) {
	s := auth.TokenScoped(ctx)
	id := int64(params.ID)
	row, err := h.sendingDomains.Update(ctx, s, id, inputs.SendingDomainUpdate(req))
	if err != nil {
		return nil, err
	}
	res := h.sendingDomainResource(row)
	return &res, nil
}

// SendingDomainsDelete deletes a Sending domain for good (ADR 0049): it leaves the list. Live
// sends from it refuse it with 409.
func (h *Handlers) SendingDomainsDelete(ctx context.Context, params externalapi.SendingDomainsDeleteParams) error {
	return h.sendingDomainDeletion.Delete(ctx, auth.TokenScoped(ctx), int64(params.ID))
}

// SendingDomainsVerify runs the live DNS check and returns the domain as the check left it
// (`verified`, `lastCheckedAt`, the envelopes).
func (h *Handlers) SendingDomainsVerify(ctx context.Context, params externalapi.SendingDomainsVerifyParams) (*externalapi.SendingDomainResource, error) {
	id := int64(params.ID)
	s := auth.TokenScoped(ctx)
	row, err := h.sendingDomains.Verify(ctx, s, id)
	if err != nil {
		return nil, err
	}
	res := h.sendingDomainResource(row)
	return &res, nil
}

// sendingDomainResource maps the View, then attaches what the mapper cannot see of the row: the
// DNS records the user publishes, which the sendingdomains module derives.
func (h *Handlers) sendingDomainResource(v sendingdomains.View) externalapi.SendingDomainResource {
	rec := h.sendingDomains.Records(v.Domain)
	res := mapper.SendingDomainToResource(v.Domain)
	res.DkimRecord = mapper.DnsRecordToResource(rec.DKIM)
	res.DmarcRecord = mapper.DnsRecordToResource(rec.DMARC)
	res.Envelopes = mapper.SendingDomainEnvelopesToResource(v.Envelopes)
	return res
}
