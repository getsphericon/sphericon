package external

import (
	"context"
	"time"

	externalapi "github.com/getsphericon/sphericon/gen/external"
	"github.com/getsphericon/sphericon/internal/api/auth"
	"github.com/getsphericon/sphericon/internal/domain/contacts/contacts"
	"github.com/getsphericon/sphericon/internal/domain/sending/reputation"
	"github.com/getsphericon/sphericon/internal/kernel/pagination"
)

// CustomFieldsList returns the whole (small, unpaginated) Custom field catalogue.
func (h *Handlers) CustomFieldsList(ctx context.Context) (*externalapi.CustomFieldsListOK, error) {
	items, err := contacts.CustomFields(ctx, auth.TokenScoped(ctx))
	if err != nil {
		return nil, err
	}
	return mapper.CustomFieldsListPage(pagination.Whole(items)), nil
}

// CustomFieldsUpdate renames a Custom field or changes its type through the Custom field retyping
// use case.
func (h *Handlers) CustomFieldsUpdate(ctx context.Context, req *externalapi.UpdateCustomFieldInput, params externalapi.CustomFieldsUpdateParams) (*externalapi.CustomFieldResource, error) {
	field, err := h.customFieldRetyping.Update(ctx, auth.TokenScoped(ctx), int64(params.ID), inputs.CustomFieldUpdate(req))
	if err != nil {
		return nil, err
	}
	res := mapper.CustomFieldToResource(field)
	return &res, nil
}

// SendingDomainRatesList returns the (numerator, denominator, rate) triples of ADR 0011
// for every Sending domain, over the trailing window.
func (h *Handlers) SendingDomainRatesList(ctx context.Context, params externalapi.SendingDomainRatesListParams) (*externalapi.SendingDomainRatesListOK, error) {
	days := params.WindowDays.Value

	rates, err := h.reputation.Rates(ctx, auth.TokenScoped(ctx), reputation.Trailing(time.Duration(days)*24*time.Hour, h.now()))
	if err != nil {
		return nil, err
	}
	return mapper.SendingDomainRatesListPage(pagination.Whole(rates), days), nil
}
