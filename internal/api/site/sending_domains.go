package site

import (
	"context"

	"github.com/getsphericon/sphericon/ent"
	siteapi "github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/domain/sending/sendingdomains"
	"github.com/getsphericon/sphericon/internal/kernel/pagination"
)

// sendingDomainResource maps the row, then attaches what the mapper cannot see: the DNS records
// the user publishes, which the sendingdomains module derives, and the envelopes of the
// Workspace's email Integrations, which its checks read (ADR 0184).
func (h *Handlers) sendingDomainResource(d *ent.SendingDomain, envelopes []sendingdomains.Envelope) siteapi.SendingDomainResource {
	rec := h.sendingDomains.Records(d)
	res := mapper.SendingDomainToResource(d)
	res.DkimRecord = mapper.DnsRecordToResource(rec.DKIM)
	res.DmarcRecord = mapper.DnsRecordToResource(rec.DMARC)
	res.Envelopes = mapper.SendingDomainEnvelopesToResource(envelopes)
	return res
}

// oneSendingDomainResource is sendingDomainResource of one row, with its envelopes read.
func (h *Handlers) oneSendingDomainResource(ctx context.Context, s *ent.Scoped, d *ent.SendingDomain) (*siteapi.SendingDomainResource, error) {
	envelopes, err := h.sendingDomains.Envelopes(ctx, s, []*ent.SendingDomain{d})
	if err != nil {
		return nil, err
	}
	res := h.sendingDomainResource(d, envelopes[d.ID])
	return &res, nil
}

func (h *Handlers) SiteSendingDomainsList(ctx context.Context, params siteapi.SiteSendingDomainsListParams) (*siteapi.SiteSendingDomainsListOK, error) {
	s := workspaceScope(ctx)

	page, err := h.sendingDomains.List(ctx, s, pagination.ParamsOf(params.Page, params.PageSize))
	if err != nil {
		return nil, err
	}
	envelopes, err := h.sendingDomains.Envelopes(ctx, s, page.Items)
	if err != nil {
		return nil, err
	}
	return mapper.SendingDomainsListPage(page.Map(func(d *ent.SendingDomain) siteapi.SendingDomainResource {
		return h.sendingDomainResource(d, envelopes[d.ID])
	})), nil
}

func (h *Handlers) SiteSendingDomainsCreate(ctx context.Context, req *siteapi.CreateSendingDomainInput, _ siteapi.SiteSendingDomainsCreateParams) (*siteapi.SendingDomainResource, error) {
	s := workspaceScope(ctx)

	d, err := h.sendingDomains.Create(ctx, s, inputs.SendingDomainCreate(req))
	if err != nil {
		return nil, err
	}
	return h.oneSendingDomainResource(ctx, s, d)
}

func (h *Handlers) SiteSendingDomainsGet(ctx context.Context, params siteapi.SiteSendingDomainsGetParams) (*siteapi.SendingDomainResource, error) {
	s := workspaceScope(ctx)

	id := int64(params.ID)
	d, err := h.sendingDomains.Get(ctx, s, id)
	if err != nil {
		return nil, err
	}
	return h.oneSendingDomainResource(ctx, s, d)
}

// SiteSendingDomainsDelete deletes a sending domain for good (ADR 0049): it leaves the list. Live
// sends from it refuse it with 409.
func (h *Handlers) SiteSendingDomainsDelete(ctx context.Context, params siteapi.SiteSendingDomainsDeleteParams) error {
	return h.sendingDomainDeletion.Delete(ctx, workspaceScope(ctx), int64(params.ID))
}

func (h *Handlers) SiteSendingDomainsVerify(ctx context.Context, params siteapi.SiteSendingDomainsVerifyParams) (*siteapi.SendingDomainResource, error) {
	s := workspaceScope(ctx)

	id := int64(params.ID)
	d, err := h.sendingDomains.Verify(ctx, s, id)
	if err != nil {
		return nil, err
	}
	return h.oneSendingDomainResource(ctx, s, d)
}
