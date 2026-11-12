package automations

import (
	"context"
	"slices"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/event"
	"github.com/getsphericon/sphericon/ent/outboundmessage"
	"github.com/getsphericon/sphericon/internal/domain/contacts/orders"
	"github.com/getsphericon/sphericon/internal/domain/sending/linkclick"
	"github.com/getsphericon/sphericon/internal/infra/aggregates"
	"github.com/getsphericon/sphericon/internal/infra/events"
	"github.com/getsphericon/sphericon/internal/kernel/systemevent"
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
	Links []linkclick.Clicks
	// Results is what the Enrollments did over the period and the period before it.
	Results Results
	// Steps is the send steps' messages by step position, in step order.
	Steps []StepFigures
}

// StepFigures is what the send step at one position sent and skipped, over every version of the
// Automation: a skip moves the Enrollment on (ADR 0081), so the report shows each step's skips.
type StepFigures struct {
	Step          int
	Sent, Skipped int
}

// ReportOf counts, leaving out machine engagement (ADR 0053), the `email.opened` Events and the click Events
// (`email.clicked`, `telegram.clicked`, `sms.clicked`) that carry the Automation, and the clicks by link, and reads its Results
// over the period p.
func (m *Module) ReportOf(ctx context.Context, s *ent.Scoped, id int64, p ReportPeriod) (Report, error) {
	if _, err := m.Get(ctx, s, id); err != nil {
		return Report{}, err
	}
	opens, err := s.Event().Query().
		Where(event.ActionEQ(systemevent.EmailOpened), events.HumanOnly(), event.AutomationID(id)).
		Count(ctx)
	if err != nil {
		return Report{}, err
	}
	links, err := linkclick.ByLink(ctx, s, linkclick.Between{}, event.AutomationID(id))
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
	steps, err := stepFiguresOf(ctx, s, id)
	if err != nil {
		return Report{}, err
	}
	report := Report{
		Opens: opens, Clicks: linkclick.Total(links), Links: links, Revenue: revenue.Of(id), Results: results, Steps: steps,
	}
	if len(delivery) > 0 {
		report.Delivered, report.Bounced, report.Complained = delivery[0].Delivered, delivery[0].Bounced, delivery[0].Complained
		report.Captured = delivery[0].Captured
	}
	return report, nil
}

// stepFiguresOf counts the Automation's sent and skipped Outbound messages by the position of the
// step that sent them, on every Channel.
func stepFiguresOf(ctx context.Context, s *ent.Scoped, id int64) ([]StepFigures, error) {
	var rows []struct {
		Step   int                    `json:"automation_step"`
		Status outboundmessage.Status `json:"status"`
		Count  int                    `json:"count"`
	}
	err := s.OutboundMessage().Query().
		Where(
			outboundmessage.AutomationID(id),
			outboundmessage.AutomationStepNotNil(),
			outboundmessage.StatusIn(outboundmessage.StatusSent, outboundmessage.StatusSkipped),
		).
		GroupBy(outboundmessage.FieldAutomationStep, outboundmessage.FieldStatus).
		Aggregate(ent.Count()).
		Scan(ctx, &rows)
	if err != nil {
		return nil, err
	}
	byStep := map[int]*StepFigures{}
	for _, r := range rows {
		f := byStep[r.Step]
		if f == nil {
			f = &StepFigures{Step: r.Step}
			byStep[r.Step] = f
		}
		if r.Status == outboundmessage.StatusSent {
			f.Sent = r.Count
		} else {
			f.Skipped = r.Count
		}
	}
	figures := make([]StepFigures, 0, len(byStep))
	for _, f := range byStep {
		figures = append(figures, *f)
	}
	slices.SortFunc(figures, func(a, b StepFigures) int { return a.Step - b.Step })
	return figures, nil
}
