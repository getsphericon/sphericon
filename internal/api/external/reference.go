package external

import (
	"context"
	"net/http"
	"time"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/customfield"
	externalapi "github.com/getsphericon/sphericon/gen/external"
	"github.com/getsphericon/sphericon/internal/api/auth"
	"github.com/getsphericon/sphericon/internal/reputation"
)

// CustomFieldsList returns the whole (small, unpaginated) Custom field catalogue.
func (h *Handlers) CustomFieldsList(ctx context.Context) (externalapi.CustomFieldsListRes, error) {
	if !auth.HasScope(auth.GetTokenAuth(ctx), externalapi.ApiTokenScopeCustomFieldsRead) {
		res := problem(http.StatusUnauthorized, "insufficient scope")
		return &res, nil
	}
	items, err := auth.TokenScoped(ctx).CustomField().Query().
		Order(ent.Asc(customfield.FieldKey)).
		All(ctx)
	if err != nil {
		return nil, err
	}
	resources := make([]externalapi.CustomFieldResource, len(items))
	for i, f := range items {
		resources[i] = mapper.CustomFieldToResource(f)
	}
	return &externalapi.CustomFieldsListOK{
		Items:      resources,
		Page:       1,
		PageSize:   int32(len(resources)),
		TotalItems: int32(len(resources)),
		TotalPages: 1,
	}, nil
}

// SendingDomainRatesList returns the (numerator, denominator, rate) triples of ADR 0011
// for every Sending domain, over the trailing window.
func (h *Handlers) SendingDomainRatesList(ctx context.Context, params externalapi.SendingDomainRatesListParams) (externalapi.SendingDomainRatesListRes, error) {
	if !auth.HasScope(auth.GetTokenAuth(ctx), externalapi.ApiTokenScopeSendingDomainsRead) {
		res := externalapi.SendingDomainRatesListUnauthorized(problem(http.StatusUnauthorized, "insufficient scope"))
		return &res, nil
	}
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
	return &externalapi.SendingDomainRatesListOK{
		Items:      items,
		Page:       1,
		PageSize:   int32(len(items)),
		TotalItems: int32(len(items)),
		TotalPages: 1,
	}, nil
}

func rateTriple(r reputation.Rate) externalapi.RateTriple {
	out := externalapi.RateTriple{Numerator: int32(r.Numerator), Denominator: int32(r.Denominator)}
	if r.Rate == nil {
		out.Rate = externalapi.NilFloat64{Null: true}
	} else {
		out.Rate = externalapi.NewNilFloat64(*r.Rate)
	}
	return out
}
