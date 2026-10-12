package external

import (
	"context"
	"errors"
	"net/http"
	"strconv"

	"github.com/samber/lo"

	"github.com/getsphericon/sphericon/ent"
	externalapi "github.com/getsphericon/sphericon/gen/external"
	"github.com/getsphericon/sphericon/internal/api/auth"
	"github.com/getsphericon/sphericon/internal/convert"
	"github.com/getsphericon/sphericon/internal/pagination"
	"github.com/getsphericon/sphericon/internal/sendingdomains"
)

// The Sending domain operations are thin adapters over internal/sendingdomains, the
// module /site shares: validation, DKIM keypair minting and sealing, and the verify
// trigger live there. The private key is never part of a response, and no operation
// can set `verified`: only the live DKIM check does.

// SendingDomainsList returns the Workspace's Sending domains with the DNS records to publish.
func (h *Handlers) SendingDomainsList(ctx context.Context, params externalapi.SendingDomainsListParams) (externalapi.SendingDomainsListRes, error) {
	if !auth.HasScope(auth.GetTokenAuth(ctx), "sending_domains:read") {
		res := externalapi.SendingDomainsListUnauthorized(problem(http.StatusUnauthorized, "insufficient scope"))
		return &res, nil
	}
	page, err := h.sendingDomains.List(ctx, auth.TokenScoped(ctx), pagination.ParamsOf(params.Page, params.PageSize))
	if err != nil {
		return nil, err
	}
	return &externalapi.SendingDomainsListOK{
		Items: lo.Map(page.Items, func(d *ent.SendingDomain, _ int) externalapi.SendingDomainResource {
			return sendingDomainResource(d)
		}),
		Page:       int32(page.Page),
		PageSize:   int32(page.PageSize),
		TotalItems: int32(page.TotalItems),
		TotalPages: int32(page.TotalPages),
	}, nil
}

// SendingDomainsCreate adds a Sending domain; the DKIM keypair is minted and sealed.
func (h *Handlers) SendingDomainsCreate(ctx context.Context, req *externalapi.CreateSendingDomainInput) (externalapi.SendingDomainsCreateRes, error) {
	if !auth.HasScope(auth.GetTokenAuth(ctx), "sending_domains:write") {
		res := externalapi.SendingDomainsCreateUnauthorized(problem(http.StatusUnauthorized, "insufficient scope"))
		return &res, nil
	}
	row, err := h.sendingDomains.Create(ctx, auth.TokenScoped(ctx), sendingdomains.CreateInput{
		Domain: req.Domain, Selector: req.DkimSelector.Or(""),
	})
	var verr *sendingdomains.ValidationError
	switch {
	case errors.As(err, &verr):
		res := externalapi.SendingDomainsCreateUnprocessableEntity(sendingDomainValidationProblem(verr))
		return &res, nil
	case errors.Is(err, sendingdomains.ErrAlreadyExists):
		res := externalapi.SendingDomainsCreateConflict(problem(http.StatusConflict, err.Error()))
		return &res, nil
	case err != nil:
		return nil, err
	}
	res := sendingDomainResource(row)
	return &res, nil
}

// SendingDomainsGet returns one Sending domain.
func (h *Handlers) SendingDomainsGet(ctx context.Context, params externalapi.SendingDomainsGetParams) (externalapi.SendingDomainsGetRes, error) {
	if !auth.HasScope(auth.GetTokenAuth(ctx), "sending_domains:read") {
		res := externalapi.SendingDomainsGetUnauthorized(problem(http.StatusUnauthorized, "insufficient scope"))
		return &res, nil
	}
	id, err := parseEntityID(params.ID)
	if err != nil {
		res := externalapi.SendingDomainsGetBadRequest(problem(http.StatusBadRequest, "invalid id"))
		return &res, nil
	}
	row, err := auth.TokenScoped(ctx).SendingDomain().Get(ctx, id)
	if ent.IsNotFound(err) {
		res := externalapi.SendingDomainsGetNotFound(problem(http.StatusNotFound, "sending domain not found"))
		return &res, nil
	}
	if err != nil {
		return nil, err
	}
	res := sendingDomainResource(row)
	return &res, nil
}

// SendingDomainsUpdate changes the DKIM selector, which unverifies the domain.
func (h *Handlers) SendingDomainsUpdate(ctx context.Context, req *externalapi.UpdateSendingDomainInput, params externalapi.SendingDomainsUpdateParams) (externalapi.SendingDomainsUpdateRes, error) {
	if !auth.HasScope(auth.GetTokenAuth(ctx), "sending_domains:write") {
		res := externalapi.SendingDomainsUpdateUnauthorized(problem(http.StatusUnauthorized, "insufficient scope"))
		return &res, nil
	}
	id, err := parseEntityID(params.ID)
	if err != nil {
		res := externalapi.SendingDomainsUpdateBadRequest(problem(http.StatusBadRequest, "invalid id"))
		return &res, nil
	}
	row, err := h.sendingDomains.Update(ctx, auth.TokenScoped(ctx), id, sendingdomains.UpdateInput{
		Selector: convert.Ptr(req.DkimSelector),
	})
	var verr *sendingdomains.ValidationError
	switch {
	case ent.IsNotFound(err):
		res := externalapi.SendingDomainsUpdateNotFound(problem(http.StatusNotFound, "sending domain not found"))
		return &res, nil
	case errors.As(err, &verr):
		res := externalapi.SendingDomainsUpdateUnprocessableEntity(sendingDomainValidationProblem(verr))
		return &res, nil
	case err != nil:
		return nil, err
	}
	res := sendingDomainResource(row)
	return &res, nil
}

// SendingDomainsDelete removes a Sending domain.
func (h *Handlers) SendingDomainsDelete(ctx context.Context, params externalapi.SendingDomainsDeleteParams) (externalapi.SendingDomainsDeleteRes, error) {
	if !auth.HasScope(auth.GetTokenAuth(ctx), "sending_domains:write") {
		res := externalapi.SendingDomainsDeleteUnauthorized(problem(http.StatusUnauthorized, "insufficient scope"))
		return &res, nil
	}
	id, err := parseEntityID(params.ID)
	if err != nil {
		res := externalapi.SendingDomainsDeleteBadRequest(problem(http.StatusBadRequest, "invalid id"))
		return &res, nil
	}
	err = h.sendingDomains.Delete(ctx, auth.TokenScoped(ctx), id)
	if ent.IsNotFound(err) {
		res := externalapi.SendingDomainsDeleteNotFound(problem(http.StatusNotFound, "sending domain not found"))
		return &res, nil
	}
	if err != nil {
		return nil, err
	}
	return &externalapi.SendingDomainsDeleteNoContent{}, nil
}

// SendingDomainsVerify triggers the live DKIM check and returns the domain as it stands;
// the outcome shows on a later read (`verified`, `lastCheckedAt`).
func (h *Handlers) SendingDomainsVerify(ctx context.Context, params externalapi.SendingDomainsVerifyParams) (externalapi.SendingDomainsVerifyRes, error) {
	if !auth.HasScope(auth.GetTokenAuth(ctx), "sending_domains:write") {
		res := externalapi.SendingDomainsVerifyUnauthorized(problem(http.StatusUnauthorized, "insufficient scope"))
		return &res, nil
	}
	id, err := parseEntityID(params.ID)
	if err != nil {
		res := externalapi.SendingDomainsVerifyBadRequest(problem(http.StatusBadRequest, "invalid id"))
		return &res, nil
	}
	s := auth.TokenScoped(ctx)
	err = h.sendingDomains.Verify(ctx, s, id)
	if ent.IsNotFound(err) {
		res := externalapi.SendingDomainsVerifyNotFound(problem(http.StatusNotFound, "sending domain not found"))
		return &res, nil
	}
	if err != nil {
		return nil, err
	}
	row, err := s.SendingDomain().Get(ctx, id)
	if err != nil {
		return nil, err
	}
	res := sendingDomainResource(row)
	return &res, nil
}

func sendingDomainValidationProblem(v *sendingdomains.ValidationError) externalapi.ProblemDetails {
	p := problem(http.StatusUnprocessableEntity, v.Message)
	p.Errors = externalapi.NewOptProblemDetailsErrors(externalapi.ProblemDetailsErrors{v.Field: {v.Message}})
	return p
}

// sendingDomainResource is hand-built (not goverter): the DNS records are derived from
// the row, and the private key is not read at all.
func sendingDomainResource(d *ent.SendingDomain) externalapi.SendingDomainResource {
	rec := sendingdomains.Records(d)
	res := externalapi.SendingDomainResource{
		ID:           externalapi.EntityId(strconv.FormatInt(d.ID, 10)),
		Domain:       d.Domain,
		DkimSelector: d.DkimSelector,
		Verified:     d.Verified,
		DkimRecord:   dnsRecord(rec.DKIM),
		SpfRecord:    dnsRecord(rec.SPF),
		DmarcRecord:  dnsRecord(rec.DMARC),
		CreatedAt:    externalapi.Timestamp(d.CreatedAt),
		UpdatedAt:    externalapi.Timestamp(d.UpdatedAt),
	}
	if d.LastCheckedAt != nil {
		res.LastCheckedAt = externalapi.NewOptNilTimestamp(externalapi.Timestamp(*d.LastCheckedAt))
	}
	if d.VerifiedAt != nil {
		res.VerifiedAt = externalapi.NewOptNilTimestamp(externalapi.Timestamp(*d.VerifiedAt))
	}
	return res
}

func dnsRecord(r sendingdomains.Record) externalapi.DnsRecord {
	return externalapi.DnsRecord{Type: externalapi.DnsRecordTypeTXT, Host: r.Host, Value: r.Value}
}
