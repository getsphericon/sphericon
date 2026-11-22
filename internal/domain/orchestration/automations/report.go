package automations

import (
	"cmp"
	"context"
	"slices"

	"github.com/samber/lo"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/automationfigure"
	"github.com/getsphericon/sphericon/ent/automationversion"
	"github.com/getsphericon/sphericon/ent/event"
	"github.com/getsphericon/sphericon/ent/outboundmessage"
	"github.com/getsphericon/sphericon/internal/domain/contacts/orders"
	"github.com/getsphericon/sphericon/internal/domain/sending/linkclick"
	"github.com/getsphericon/sphericon/internal/infra/aggregates"
	"github.com/getsphericon/sphericon/internal/infra/db"
	"github.com/getsphericon/sphericon/internal/infra/events"
	"github.com/getsphericon/sphericon/internal/kernel/automationstep"
	"github.com/getsphericon/sphericon/internal/kernel/systemevent"
)

// Report is an Automation's engagement figures and its Results over a period. Every open and
// click is an Event (ADR 0053), so these are repeat counts, and they cover the Event retention
// window; the engagement figures are not cut to the period.
type Report struct {
	Opens, Clicks int
	// Delivered, Bounced and Complained count the Automation's Outbound messages by their
	// delivery result stamp (ADR 0053), so they do not shrink when Events expire, and add what its
	// pruned messages counted (ADR 0143).
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
	// Steps is the send Steps' messages by Step id, each summed over the versions that had it
	// (ADR 0122), sorted by the newest of those versions and then by its place there.
	Steps []StepFigures
	// VersionSteps is the send Steps' messages by version and Step id, in version order and, within
	// a version, in its Step order.
	VersionSteps []VersionStepFigures
}

// StepFigures is what one send Step sent and skipped over every version of the Automation that had
// it: a skip moves the Enrollment on (ADR 0081), so the report shows each Step's skips.
type StepFigures struct {
	StepID        automationstep.ID
	Sent, Skipped int
}

// VersionStepFigures is what one send Step of one version sent and skipped; Version is the
// version's number.
type VersionStepFigures struct {
	Version       int
	StepID        automationstep.ID
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
	links, err := m.clicks.ByLink(ctx, s, linkclick.Between{}, event.AutomationID(id))
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
	revenue, err := m.revenues.CreditedToAutomations(ctx, s, id)
	if err != nil {
		return Report{}, err
	}
	results, err := m.resultsOf(ctx, s, id, p)
	if err != nil {
		return Report{}, err
	}
	sent, err := sentByStep(ctx, s, id)
	if err != nil {
		return Report{}, err
	}
	report := Report{
		Opens: opens, Clicks: linkclick.Total(links), Links: links, Revenue: revenue.Of(id), Results: results,
	}
	if len(delivery) > 0 {
		report.Delivered, report.Bounced, report.Complained = delivery[0].Delivered, delivery[0].Bounced, delivery[0].Complained
		report.Captured = delivery[0].Captured
	}
	if err := addPruned(ctx, s, id, &report, sent); err != nil {
		return Report{}, err
	}
	if err := stepFiguresOf(ctx, s, id, sent, &report); err != nil {
		return Report{}, err
	}
	return report, nil
}

// versionStep names a send Step of one version: the version's row id and the Step's id.
type versionStep struct {
	versionID int64
	stepID    automationstep.ID
}

// sentSkipped is what a send Step sent and skipped.
type sentSkipped struct{ sent, skipped int }

// addPruned adds to the report what the Automation's pruned messages counted (ADR 0143): their
// delivery stamps and, into sent, their sent and skipped by version and Step id.
func addPruned(ctx context.Context, s *ent.Scoped, id int64, report *Report, sent map[versionStep]*sentSkipped) error {
	pruned, err := s.AutomationFigure().Query().Where(automationfigure.AutomationID(id)).All(ctx)
	if err != nil {
		return err
	}
	for _, f := range pruned {
		report.Delivered += f.Delivered
		report.Bounced += f.Bounced
		report.Complained += f.Complained
		report.Captured += f.Captured
		c := countsOf(sent, versionStep{versionID: f.VersionID, stepID: f.StepID})
		c.sent += f.Sent
		c.skipped += f.Skipped
	}
	return nil
}

// countsOf is the counts of the Step in sent, added at zero when it has none yet.
func countsOf(sent map[versionStep]*sentSkipped, k versionStep) *sentSkipped {
	c := sent[k]
	if c == nil {
		c = &sentSkipped{}
		sent[k] = c
	}
	return c
}

// sentByStep counts the Automation's sent and skipped Outbound messages by the version and the
// Step id that sent them, on every Channel.
func sentByStep(ctx context.Context, s *ent.Scoped, id int64) (map[versionStep]*sentSkipped, error) {
	var rows []struct {
		VersionID int64                  `json:"automation_version_id"`
		StepID    automationstep.ID      `json:"automation_step_id"`
		Status    outboundmessage.Status `json:"status"`
		Count     int                    `json:"count"`
	}
	err := s.OutboundMessage().Query().
		Where(
			outboundmessage.AutomationID(id),
			outboundmessage.StatusIn(outboundmessage.StatusSent, outboundmessage.StatusSkipped),
		).
		GroupBy(outboundmessage.FieldAutomationVersionID, outboundmessage.FieldAutomationStepID, outboundmessage.FieldStatus).
		Aggregate(ent.Count()).
		Scan(ctx, &rows)
	if err != nil {
		return nil, err
	}
	sent := map[versionStep]*sentSkipped{}
	for _, r := range rows {
		c := countsOf(sent, versionStep{versionID: r.VersionID, stepID: r.StepID})
		if r.Status == outboundmessage.StatusSent {
			c.sent = r.Count
		} else {
			c.skipped = r.Count
		}
	}
	return sent, nil
}

// stepFiguresOf sets the report's Steps and VersionSteps from what each version's send Steps
// sent: per version in version order, then each Step's place in its version, a Step the version
// no longer holds after the ones it does; and summed by Step id, in the order of the newest version
// that had each Step.
func stepFiguresOf(ctx context.Context, s *ent.Scoped, id int64, sent map[versionStep]*sentSkipped, report *Report) error {
	versionIDs := lo.Uniq(lo.Map(lo.Keys(sent), func(k versionStep, _ int) int64 { return k.versionID }))
	versions, err := s.AutomationVersion().Query().
		Where(automationversion.AutomationID(id), automationversion.IDIn(versionIDs...)).
		Order(ent.Asc(automationversion.FieldVersion)).
		All(ctx)
	if err != nil {
		return err
	}
	byVersion := make([][]VersionStepFigures, 0, len(versions))
	for _, v := range versions {
		place := map[automationstep.ID]int{}
		for pos, st := range automationstep.All(v.Steps) {
			place[st.ID] = pos
		}
		var figures []VersionStepFigures
		for k, c := range sent {
			if k.versionID == v.ID {
				figures = append(figures, VersionStepFigures{Version: v.Version, StepID: k.stepID, Sent: c.sent, Skipped: c.skipped})
			}
		}
		slices.SortFunc(figures, func(a, b VersionStepFigures) int {
			return cmp.Or(cmp.Compare(placeOf(place, a.StepID), placeOf(place, b.StepID)), cmp.Compare(a.StepID, b.StepID))
		})
		byVersion = append(byVersion, figures)
	}
	report.VersionSteps = append([]VersionStepFigures{}, slices.Concat(byVersion...)...)
	report.Steps = []StepFigures{}
	sums := map[automationstep.ID]int{}
	for _, figures := range slices.Backward(byVersion) {
		for _, f := range figures {
			i, ok := sums[f.StepID]
			if !ok {
				i = len(report.Steps)
				sums[f.StepID] = i
				report.Steps = append(report.Steps, StepFigures{StepID: f.StepID})
			}
			report.Steps[i].Sent += f.Sent
			report.Steps[i].Skipped += f.Skipped
		}
	}
	return nil
}

// placeOf is the Step's place in a version, after every Step it holds when it holds none of that
// id.
func placeOf(place map[automationstep.ID]int, stepID automationstep.ID) int {
	if pos, ok := place[stepID]; ok {
		return pos
	}
	return len(place)
}

// The unique constraint of an Automation's pruned figures (ADR 0106): no client can break it.
var _ = db.Declare(
	db.Fault("automation_figures_workspace_automation_version_step",
		"automationsunchecked.Module.AddPruned upserts ON CONFLICT on the version and the Step, adding a batch's counts to the row"),
)
