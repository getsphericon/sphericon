package site

import (
	"context"

	siteapi "github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/domain/sending/sendingdomains"
	"github.com/getsphericon/sphericon/internal/kernel/pagination"
)

// sendingDomainResource maps the View, then attaches what the mapper cannot see of the row: the
// DNS records the user publishes, which the sendingdomains module derives.
func (h *Handlers) sendingDomainResource(v sendingdomains.View) siteapi.SendingDomainResource {
	rec := h.sendingDomains.Records(v.Domain)
	res := mapper.SendingDomainToResource(v.Domain)
	res.DkimRecord = mapper.DnsRecordToResource(rec.DKIM)
	res.DmarcRecord = mapper.DnsRecordToResource(rec.DMARC)
	res.Envelopes = mapper.SendingDomainEnvelopesToResource(v.Envelopes)
	return res
}

func (h *Handlers) SiteSendingDomainsList(ctx context.Context, params siteapi.SiteSendingDomainsListParams) (*siteapi.SiteSendingDomainsListOK, error) {
	s := workspaceScope(ctx)

	page, err := h.sendingDomains.List(ctx, s, pagination.ParamsOf(params.Page, params.PageSize))
	if err != nil {
		return nil, err
	}
	return mapper.SendingDomainsListPage(page.Map(h.sendingDomainResource)), nil
}

func (h *Handlers) SiteSendingDomainsCreate(ctx context.Context, req *siteapi.CreateSendingDomainInput, _ siteapi.SiteSendingDomainsCreateParams) (*siteapi.SendingDomainResource, error) {
	s := workspaceScope(ctx)

	d, err := h.sendingDomains.Create(ctx, s, inputs.SendingDomainCreate(req))
	if err != nil {
		return nil, err
	}
	res := h.sendingDomainResource(d)
	return &res, nil
}

func (h *Handlers) SiteSendingDomainsGet(ctx context.Context, params siteapi.SiteSendingDomainsGetParams) (*siteapi.SendingDomainResource, error) {
	s := workspaceScope(ctx)

	id := int64(params.ID)
	d, err := h.sendingDomains.Get(ctx, s, id)
	if err != nil {
		return nil, err
	}
	res := h.sendingDomainResource(d)
	return &res, nil
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
	res := h.sendingDomainResource(d)
	return &res, nil
}
