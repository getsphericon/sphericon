package analyticsreport

import (
	"context"
	"time"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/internal/reputation"
)

// OverviewDeliverabilityWindow is the trailing window of the overview's deliverability: it does
// not follow the overview's range.
const OverviewDeliverabilityWindow = 24 * time.Hour

// DomainRates are the Complaint and Bounce rates of one Sending domain, as the (numerator,
// denominator, rate) triples of ADR 0011: Complaint is complaints over delivered, Bounce is hard
// bounces over sent.
type DomainRates struct {
	SendingDomainID   int64
	Domain            string
	Complaint, Bounce reputation.Rate
}

// Deliverability is the Sending domains' rates over a window.
type Deliverability struct {
	Window  time.Duration
	Domains []DomainRates
}

// Deliverability reports the rates of the Sending domains that had sends or deliveries in the
// window, archived ones included (ADR 0049). The rates are those of internal/reputation, the one
// place that counts; the core shows them as they are, with no display floor and no limit.
func (m *UseCase) Deliverability(ctx context.Context, s *ent.Scoped, w Window) (*Deliverability, error) {
	days, err := m.days(ctx, s, w)
	if err != nil {
		return nil, err
	}
	return m.deliverability(ctx, s, reputation.Window{Since: days.Since, Until: days.Until}, time.Duration(w)*24*time.Hour)
}

// overviewDeliverability is the overview's cut: the trailing 24 hours ending now.
func (m *UseCase) overviewDeliverability(ctx context.Context, s *ent.Scoped) (Deliverability, error) {
	now := m.now()
	d, err := m.deliverability(ctx, s, reputation.Window{Since: now.Add(-OverviewDeliverabilityWindow), Until: now}, OverviewDeliverabilityWindow)
	if err != nil {
		return Deliverability{}, err
	}
	return *d, nil
}

func (m *UseCase) deliverability(ctx context.Context, s *ent.Scoped, w reputation.Window, length time.Duration) (*Deliverability, error) {
	rates, err := reputation.New().Rates(ctx, s, w)
	if err != nil {
		return nil, err
	}
	out := &Deliverability{Window: length, Domains: make([]DomainRates, 0, len(rates))}
	for _, r := range rates {
		// A domain with no traffic in the window is left out. A complaint or bounce of mail sent
		// before the window is traffic too: it is listed, with its rate undefined.
		if r.Bounce.Numerator+r.Bounce.Denominator+r.Complaint.Numerator+r.Complaint.Denominator == 0 {
			continue
		}
		out.Domains = append(out.Domains, DomainRates{
			SendingDomainID: r.Domain.ID,
			Domain:          r.Domain.Domain,
			Complaint:       r.Complaint,
			Bounce:          r.Bounce,
		})
	}
	return out, nil
}
