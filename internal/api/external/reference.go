package external

import (
	"context"
	"time"

	externalapi "github.com/getsphericon/sphericon/gen/external"
	"github.com/getsphericon/sphericon/internal/api/auth"
	"github.com/getsphericon/sphericon/internal/contacts"
	"github.com/getsphericon/sphericon/internal/narrow"
	"github.com/getsphericon/sphericon/internal/pagination"
	"github.com/getsphericon/sphericon/internal/reputation"
)

// CustomFieldsList returns the whole (small, unpaginated) Custom field catalogue.
func (h *Handlers) CustomFieldsList(ctx context.Context) (externalapi.CustomFieldsListRes, error) {
	items, err := contacts.CustomFields(ctx, auth.TokenScoped(ctx))
	if err != nil {
		return nil, err
	}
	return mapper.CustomFieldsListPage(pagination.Whole(items)), nil
}

// SendingDomainRatesList returns the (numerator, denominator, rate) triples of ADR 0011
// for every Sending domain, over the trailing window.
func (h *Handlers) SendingDomainRatesList(ctx context.Context, params externalapi.SendingDomainRatesListParams) (externalapi.SendingDomainRatesListRes, error) {
	days := params.WindowDays.Value

	rates, err := h.reputation.Rates(ctx, auth.TokenScoped(ctx), reputation.Trailing(time.Duration(days)*24*time.Hour))
	if err != nil {
		return nil, err
	}
	items := make([]externalapi.SendingDomainRatesResource, len(rates))
	for i, r := range rates {
		items[i] = externalapi.SendingDomainRatesResource{
			SendingDomainId: externalapi.EntityId(r.Domain.ID),
			Domain:          r.Domain.Domain,
			WindowDays:      days,
			ComplaintRate:   rateTriple(r.Complaint),
			BounceRate:      rateTriple(r.Bounce),
		}
	}
	return mapper.SendingDomainRatesListPage(pagination.Whole(items)), nil
}

func rateTriple(r reputation.Rate) externalapi.RateTriple {
	out := externalapi.RateTriple{Numerator: narrow.Int32(r.Numerator), Denominator: narrow.Int32(r.Denominator)}
	if r.Rate == nil {
		out.Rate = externalapi.NilFloat64{Null: true}
	} else {
		out.Rate = externalapi.NewNilFloat64(*r.Rate)
	}
	return out
}
