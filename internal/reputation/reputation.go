// Package reputation computes the deliverability rates of ADR 0011: complaint and
// bounce rate per (Workspace, Sending domain) over a trailing window.
//
// The rates are flow rates by each Event's own timestamp, computed by a live windowed
// query over events (no rollup table). Each is exposed as the (numerator, denominator,
// rate) triple; the rate is undefined (nil) when the denominator is zero.
package reputation

import (
	"context"
	"time"

	"entgo.io/ent/dialect/sql"
	"entgo.io/ent/dialect/sql/sqljson"
	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/event"
	"github.com/getsphericon/sphericon/ent/predicate"
	"github.com/getsphericon/sphericon/ent/sendingdomain"
	"github.com/getsphericon/sphericon/internal/events"
)

// Rate is a numerator over a denominator. Rate is nil when Denominator is zero.
type Rate struct {
	Numerator   int
	Denominator int
	Rate        *float64
}

func newRate(numerator, denominator int) Rate {
	r := Rate{Numerator: numerator, Denominator: denominator}
	if denominator > 0 {
		v := float64(numerator) / float64(denominator)
		r.Rate = &v
	}
	return r
}

// DomainRates are the rates of one Sending domain.
type DomainRates struct {
	Domain *ent.SendingDomain
	// Complaint is complaints / (sent - hard bounces): a hard bounce was never delivered.
	Complaint Rate
	// Bounce is hard bounces / sent. Soft (transient) bounces are not counted.
	Bounce Rate
}

// Window is the span the rates are over: events from Since up to, not including, Until.
type Window struct {
	Since, Until time.Time
}

// Trailing is the window of the given length ending now.
func Trailing(d time.Duration) Window {
	now := time.Now()
	return Window{Since: now.Add(-d), Until: now}
}

// Module reads the rates.
type Module struct{}

// New builds the module.
func New() *Module {
	return &Module{}
}

// Rates returns the rates of every Sending domain of the workspace, archived ones too (ADR 0049: archiving never changes past figures) over the window. A domain with no traffic is still listed, with undefined rates.
func (m *Module) Rates(ctx context.Context, s *ent.Scoped, w Window) ([]DomainRates, error) {
	domains, err := s.SendingDomain().QueryWithDiscarded().
		Order(ent.Asc(sendingdomain.FieldID)).
		All(ctx)
	if err != nil {
		return nil, err
	}

	out := make([]DomainRates, 0, len(domains))
	for _, d := range domains {
		count := func(action string, extra ...predicate.Event) (int, error) {
			preds := append([]predicate.Event{
				event.Action(action),
				inWindow(w), onDomain(d.Domain),
			}, extra...)
			return s.Event().Query().Where(preds...).Count(ctx)
		}
		sent, err := count(events.NameEmailSent)
		if err != nil {
			return nil, err
		}
		hard, err := count(events.NameEmailBounced, hardBounce())
		if err != nil {
			return nil, err
		}
		complaints, err := count(events.NameEmailComplained)
		if err != nil {
			return nil, err
		}
		out = append(out, DomainRates{
			Domain:    d,
			Complaint: newRate(complaints, sent-hard),
			Bounce:    newRate(hard, sent),
		})
	}
	return out, nil
}

// inWindow selects Events by their own occurrence time (falling back to ingest time).
func inWindow(w Window) predicate.Event {
	return func(s *sql.Selector) {
		at := "COALESCE(" + s.C(event.FieldOccurredAt) + ", " + s.C(event.FieldCreatedAt) + ")"
		// raw sql: a range over COALESCE(occurred_at, created_at); ent has no COALESCE operand in a
		// predicate.
		s.Where(sql.P(func(b *sql.Builder) {
			b.WriteString(at)
			b.WriteOp(sql.OpGTE).Arg(w.Since)
			b.WriteString(" AND " + at)
			b.WriteOp(sql.OpLT).Arg(w.Until)
		}))
	}
}

func onDomain(domain string) predicate.Event {
	return func(s *sql.Selector) {
		s.Where(sqljson.ValueEQ(event.FieldProperties, domain, sqljson.Path("sendingDomain")))
	}
}

func hardBounce() predicate.Event {
	return func(s *sql.Selector) {
		s.Where(sqljson.ValueEQ(event.FieldProperties, events.BounceKindPermanent, sqljson.Path("bounceKind")))
	}
}
