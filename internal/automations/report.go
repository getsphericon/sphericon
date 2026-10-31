package automations

import (
	"context"

	"entgo.io/ent/dialect/sql"
	"entgo.io/ent/dialect/sql/sqljson"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/event"
	"github.com/getsphericon/sphericon/ent/outboundmessage"
	"github.com/getsphericon/sphericon/internal/aggregates"
	"github.com/getsphericon/sphericon/internal/events"
	"github.com/getsphericon/sphericon/internal/linkclicks"
	"github.com/getsphericon/sphericon/internal/orders"
)

// Report is an Automation's engagement figures and its Results over a period. Every open and
// click is an Event (ADR 0053), so these are repeat counts, and they cover the Event retention
// window; the engagement figures are not cut to the period.
type Report struct {
	Opens, Clicks int
	// Delivered, Bounced and Complained count the Automation's Outbound messages by their
	// delivery result stamp (ADR 0053), so they do not shrink when Events expire.
	Delivered, Bounced, Complained int
	// Captured counts the messages that went through a sender delivering nowhere outside (a Test
	// inbox): captured, not delivered.
	Captured int
	// Revenue is the Orders credited to the Automation and their net revenue in the Reporting
	// currency (ADR 0064), nil Currency until it is chosen.
	Revenue orders.Revenue
	// Links counts the clicks by link, most clicked first.
	Links []linkclicks.Clicks
	// Results is what the Enrollments did over the period and the period before it.
	Results Results
}

// ReportOf counts, leaving out machine engagement (ADR 0053), the `email.opened` and
// `email.clicked` Events that carry the Automation, and the clicks by link, and reads its Results
// over the period p.
func (m *Module) ReportOf(ctx context.Context, s *ent.Scoped, id int64, p ReportPeriod) (Report, error) {
	if _, err := m.Get(ctx, s, id); err != nil {
		return Report{}, err
	}
	ofAutomation := func(sel *sql.Selector) {
		sel.Where(sqljson.ValueEQ(event.FieldProperties, id, sqljson.Path("automationId")))
	}
	opens, err := s.Event().Query().
		Where(event.ActionEQ(events.NameEmailOpened)).
		Modify(events.HumanOnly, ofAutomation).
		Count(ctx)
	if err != nil {
		return Report{}, err
	}
	links, err := linkclicks.ByLink(ctx, s, linkclicks.Between{}, ofAutomation)
	if err != nil {
		return Report{}, err
	}
	var delivery []struct {
		Delivered  int `json:"delivered"`
		Bounced    int `json:"bounced"`
		Complained int `json:"complained"`
		Captured   int `json:"captured"`
	}
	err = s.OutboundMessage().Query().
		Where(outboundmessage.AutomationID(id)).
		Aggregate(
			aggregates.CountSet(outboundmessage.FieldDeliveredAt, "delivered"),
			aggregates.CountSet(outboundmessage.FieldBouncedAt, "bounced"),
			aggregates.CountSet(outboundmessage.FieldComplainedAt, "complained"),
			aggregates.CountTrue(outboundmessage.FieldCaptured, "captured"),
		).
		Scan(ctx, &delivery)
	if err != nil {
		return Report{}, err
	}
	revenue, err := orders.CreditedToAutomations(ctx, s, id)
	if err != nil {
		return Report{}, err
	}
	results, err := m.resultsOf(ctx, s, id, p)
	if err != nil {
		return Report{}, err
	}
	report := Report{Opens: opens, Clicks: linkclicks.Total(links), Links: links, Revenue: revenue.Of(id), Results: results}
	if len(delivery) > 0 {
		report.Delivered, report.Bounced, report.Complained = delivery[0].Delivered, delivery[0].Bounced, delivery[0].Complained
		report.Captured = delivery[0].Captured
	}
	return report, nil
}
