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
)

// Report is an Automation's engagement figures. Every open and click is an Event (ADR 0053), so
// these are repeat counts, and they cover the Event retention window.
type Report struct {
	Opens, Clicks int
	// Delivered, Bounced and Complained count the Automation's Outbound messages by their
	// delivery result stamp (ADR 0053), so they do not shrink when Events expire.
	Delivered, Bounced, Complained int
	// Links counts the clicks by link, most clicked first.
	Links []linkclicks.Clicks
}

// ReportOf counts, leaving out machine engagement (ADR 0053), the `email.opened` and
// `email.clicked` Events that carry the Automation, and the clicks by link.
func (m *Module) ReportOf(ctx context.Context, s *ent.Scoped, id int64) (Report, error) {
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
	var results []struct {
		Delivered  int `json:"delivered"`
		Bounced    int `json:"bounced"`
		Complained int `json:"complained"`
	}
	err = s.OutboundMessage().Query().
		Where(outboundmessage.AutomationID(id)).
		Aggregate(
			aggregates.CountSet(outboundmessage.FieldDeliveredAt, "delivered"),
			aggregates.CountSet(outboundmessage.FieldBouncedAt, "bounced"),
			aggregates.CountSet(outboundmessage.FieldComplainedAt, "complained"),
		).
		Scan(ctx, &results)
	if err != nil {
		return Report{}, err
	}
	report := Report{Opens: opens, Clicks: linkclicks.Total(links), Links: links}
	if len(results) > 0 {
		report.Delivered, report.Bounced, report.Complained = results[0].Delivered, results[0].Bounced, results[0].Complained
	}
	return report, nil
}
