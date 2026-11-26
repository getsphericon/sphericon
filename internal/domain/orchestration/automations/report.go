package automations

import (
	"cmp"
	"context"
	"slices"

	"entgo.io/ent/dialect/sql"
	"github.com/samber/lo"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/automationfigure"
	"github.com/getsphericon/sphericon/ent/automationrun"
	"github.com/getsphericon/sphericon/ent/automationsteprecord"
	"github.com/getsphericon/sphericon/ent/automationversion"
	"github.com/getsphericon/sphericon/ent/event"
	"github.com/getsphericon/sphericon/ent/outboundmessage"
	"github.com/getsphericon/sphericon/internal/domain/contacts/orders"
	"github.com/getsphericon/sphericon/internal/domain/sending/linkclick"
	"github.com/getsphericon/sphericon/internal/infra/aggregates"
	"github.com/getsphericon/sphericon/internal/infra/db"
	"github.com/getsphericon/sphericon/internal/infra/events"
	"github.com/getsphericon/sphericon/internal/kernel/automationstep"
	"github.com/getsphericon/sphericon/internal/kernel/deferralcause"
	"github.com/getsphericon/sphericon/internal/kernel/reportday"
	"github.com/getsphericon/sphericon/internal/kernel/skipreason"
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
	// Paths is the Enrollments that took each path of each fork, by the fork's Step id and the
	// path's id, each summed over the versions that had the path (ADR 0124), sorted by the newest
	// of those versions, then by the fork's place there and the path's place in the fork.
	Paths []PathFigures
	// WaitUntilSteps is the Enrollments each Wait until let go on at once in the current period,
	// by its Step id and why (ADR 0126), in Step id order.
	WaitUntilSteps []WaitUntilFigures
}

// WaitUntilFigures is how many Enrollments went on at once from one Wait until on a date, over
// every version that had it, by why: its moment had passed, the Contact held no date, or the
// moment lay more than 366 days ahead.
type WaitUntilFigures struct {
	StepID                    automationstep.ID
	Past, NoDate, BeyondBound int
}

// PathFigures is how many Enrollments took one path of a fork, over every version of the
// Automation that had it, and how many of them reached the goal.
type PathFigures struct {
	StepID, PathID           automationstep.ID
	Enrollments, GoalReached int
}

// StepFigures is what one send Step sent and skipped over every version of the Automation that had
// it: a skip moves the Enrollment on (ADR 0081), so the report shows each Step's skips, by their
// Skipped reason, and the active Enrollments whose send there waits now, by the cause of its latest
// Deferral (ADR 0178). Both name every reason and cause of the closed lists. SingleZone counts the
// sent messages a Sending window judged by a single zone (ADR 0134).
type StepFigures struct {
	StepID          automationstep.ID
	Sent, Skipped   int
	SkippedByReason []skipreason.Count
	WaitingByCause  []deferralcause.Count
	SingleZone      int
}

// VersionStepFigures is StepFigures for one send Step of one version; Version is the version's
// number.
type VersionStepFigures struct {
	Version         int
	StepID          automationstep.ID
	Sent, Skipped   int
	SkippedByReason []skipreason.Count
	WaitingByCause  []deferralcause.Count
	SingleZone      int
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
	if err := waitingByStep(ctx, s, id, sent); err != nil {
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
	taken, err := pathsTaken(ctx, s, id)
	if err != nil {
		return Report{}, err
	}
	if report.Paths, err = pathFiguresOf(ctx, s, id, taken); err != nil {
		return Report{}, err
	}
	period, err := m.periodOf(ctx, s, p)
	if err != nil {
		return Report{}, err
	}
	if report.WaitUntilSteps, err = wentOnByStep(ctx, s, id, period); err != nil {
		return Report{}, err
	}
	return report, nil
}

// wentOnByStep counts the Automation's Enrollments that went on at once from a Wait until (ADR
// 0126), by its Step id and why, as their Step records say: each counted on the moment it left the
// Step, within the window w. A record left by an end of the Enrollment before its job ran has no
// outcome and is not counted: the Enrollment never went on.
func wentOnByStep(ctx context.Context, s *ent.Scoped, id int64, w reportday.Window) ([]WaitUntilFigures, error) {
	var rows []struct {
		StepID automationstep.ID     `sql:"step_id"`
		WentOn automationstep.WentOn `sql:"went_on"`
		N      int                   `sql:"n"`
	}
	err := s.AutomationStepRecord().Query().
		Where(
			automationsteprecord.WentOnNotNil(),
			automationsteprecord.OutcomeEQ(automationsteprecord.OutcomeDone),
			automationsteprecord.LeftAtGTE(w.Since), automationsteprecord.LeftAtLT(w.Until),
		).
		Modify(func(sel *sql.Selector) {
			run := sql.Table(automationrun.Table)
			// The run's foreign key is composite: joined on both columns, the runs stay in the
			// scope's Workspace and the read takes the runs' Workspace and Automation index.
			sel.Join(run).OnP(sql.And(
				sql.ColumnsEQ(sel.C(automationsteprecord.FieldWorkspaceID), run.C(automationrun.FieldWorkspaceID)),
				sql.ColumnsEQ(sel.C(automationsteprecord.FieldRunID), run.C(automationrun.FieldID)),
			))
			sel.Where(sql.EQ(run.C(automationrun.FieldAutomationID), id))
			sel.Select(
				sql.As(sel.C(automationsteprecord.FieldStepID), "step_id"),
				sql.As(sel.C(automationsteprecord.FieldWentOn), "went_on"),
				sql.As(sql.Count("*"), "n"),
			).GroupBy(sel.C(automationsteprecord.FieldStepID), sel.C(automationsteprecord.FieldWentOn))
		}).Scan(ctx, &rows)
	if err != nil {
		return nil, err
	}
	figures := []WaitUntilFigures{}
	at := map[automationstep.ID]int{}
	for _, r := range rows {
		i, ok := at[r.StepID]
		if !ok {
			i = len(figures)
			at[r.StepID] = i
			figures = append(figures, WaitUntilFigures{StepID: r.StepID})
		}
		switch r.WentOn {
		case automationstep.WentOnPast:
			figures[i].Past += r.N
		case automationstep.WentOnNoDate:
			figures[i].NoDate += r.N
		case automationstep.WentOnBeyondBound:
			figures[i].BeyondBound += r.N
		}
	}
	slices.SortFunc(figures, func(a, b WaitUntilFigures) int { return cmp.Compare(a.StepID, b.StepID) })
	return figures, nil
}

// versionStep names a send Step of one version: the version's row id and the Step's id.
type versionStep struct {
	versionID int64
	stepID    automationstep.ID
}

// sentSkipped is what a send Step sent and skipped, its skips by reason, the active Enrollments
// whose send waits there now by cause, and the sent messages a Sending window judged by a single
// zone.
type sentSkipped struct {
	sent, skipped, singleZone int
	skippedBy                 map[skipreason.Reason]int
	waiting                   map[deferralcause.Cause]int
}

// add adds o's counts to c's.
func (c *sentSkipped) add(o *sentSkipped) {
	c.sent += o.sent
	c.skipped += o.skipped
	c.singleZone += o.singleZone
	for r, n := range o.skippedBy {
		c.skip(r, n)
	}
	for cause, n := range o.waiting {
		c.wait(cause, n)
	}
}

// skip counts n sends Skipped for the reason r.
func (c *sentSkipped) skip(r skipreason.Reason, n int) {
	if c.skippedBy == nil {
		c.skippedBy = map[skipreason.Reason]int{}
	}
	c.skippedBy[r] += n
}

// wait counts n Enrollments waiting for the cause.
func (c *sentSkipped) wait(cause deferralcause.Cause, n int) {
	if c.waiting == nil {
		c.waiting = map[deferralcause.Cause]int{}
	}
	c.waiting[cause] += n
}

// addPruned adds to the report what the Automation's pruned messages counted (ADR 0143): their
// delivery stamps and, into sent, their sent and skipped, by reason too, by version and Step id.
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
		c.add(&sentSkipped{sent: f.Sent, skipped: f.Skipped, singleZone: f.SingleZone, skippedBy: map[skipreason.Reason]int{
			skipreason.Suppressed:             f.SkippedSuppressed,
			skipreason.UnsubscribedEverything: f.SkippedUnsubscribedEverything,
			skipreason.UnsubscribedSource:     f.SkippedUnsubscribedSource,
			skipreason.Unconfirmed:            f.SkippedUnconfirmed,
			skipreason.NoDestination:          f.SkippedNoDestination,
			skipreason.ContactErased:          f.SkippedContactErased,
			skipreason.NoLocalTime:            f.SkippedNoLocalTime,
			skipreason.FrequencyCap:           f.SkippedFrequencyCap,
		}})
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
// Step id that sent them, on every Channel, the skipped ones by their Skipped reason too.
func sentByStep(ctx context.Context, s *ent.Scoped, id int64) (map[versionStep]*sentSkipped, error) {
	var rows []struct {
		VersionID int64                  `json:"automation_version_id"`
		StepID    automationstep.ID      `json:"automation_step_id"`
		Status    outboundmessage.Status `json:"status"`
		Reason    *skipreason.Reason     `json:"skipped_reason"`
		Count     int                    `json:"count"`
		Single    int                    `json:"single_zone"`
	}
	err := s.OutboundMessage().Query().
		Where(
			outboundmessage.AutomationID(id),
			outboundmessage.StatusIn(outboundmessage.StatusSent, outboundmessage.StatusSkipped),
		).
		GroupBy(outboundmessage.FieldAutomationVersionID, outboundmessage.FieldAutomationStepID, outboundmessage.FieldStatus, outboundmessage.FieldSkippedReason).
		Aggregate(ent.Count(), aggregates.CountTrue(outboundmessage.FieldWindowSingleZone, "single_zone")).
		Scan(ctx, &rows)
	if err != nil {
		return nil, err
	}
	sent := map[versionStep]*sentSkipped{}
	for _, r := range rows {
		c := countsOf(sent, versionStep{versionID: r.VersionID, stepID: r.StepID})
		if r.Status == outboundmessage.StatusSent {
			c.sent += r.Count
			c.singleZone += r.Single
			continue
		}
		c.skipped += r.Count
		// The outcome check sets a reason on every skipped message.
		c.skip(lo.FromPtr(r.Reason), r.Count)
	}
	return sent, nil
}

// waitingByStep counts into sent the Automation's active Enrollments whose send waits now (ADR
// 0178), by the version and the Step id they stand on and the cause of the latest Deferral there:
// the move off the Step and the end of the Enrollment clear it.
func waitingByStep(ctx context.Context, s *ent.Scoped, id int64, sent map[versionStep]*sentSkipped) error {
	var rows []struct {
		VersionID int64               `json:"version_id"`
		StepID    automationstep.ID   `json:"current_step_id"`
		Cause     deferralcause.Cause `json:"deferral_cause"`
		Count     int                 `json:"count"`
	}
	err := s.AutomationRun().Query().
		Where(
			automationrun.AutomationID(id),
			automationrun.StatusEQ(automationrun.StatusActive),
			automationrun.DeferralCauseNotNil(),
			automationrun.CurrentStepIDNotNil(),
		).
		GroupBy(automationrun.FieldVersionID, automationrun.FieldCurrentStepID, automationrun.FieldDeferralCause).
		Aggregate(ent.Count()).
		Scan(ctx, &rows)
	if err != nil {
		return err
	}
	for _, r := range rows {
		countsOf(sent, versionStep{versionID: r.VersionID, stepID: r.StepID}).wait(r.Cause, r.Count)
	}
	return nil
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
				figures = append(figures, VersionStepFigures{
					Version: v.Version, StepID: k.stepID, Sent: c.sent, Skipped: c.skipped, SingleZone: c.singleZone,
					SkippedByReason: skipreason.Counts(c.skippedBy), WaitingByCause: deferralcause.Counts(c.waiting),
				})
			}
		}
		slices.SortFunc(figures, func(a, b VersionStepFigures) int {
			return cmp.Or(cmp.Compare(placeOf(place, a.StepID), placeOf(place, b.StepID)), cmp.Compare(a.StepID, b.StepID))
		})
		byVersion = append(byVersion, figures)
	}
	report.VersionSteps = append([]VersionStepFigures{}, slices.Concat(byVersion...)...)
	versionOf := lo.SliceToMap(versions, func(v *ent.AutomationVersion) (int, int64) { return v.Version, v.ID })
	var order []automationstep.ID
	sums := map[automationstep.ID]*sentSkipped{}
	for _, figures := range slices.Backward(byVersion) {
		for _, f := range figures {
			sum, ok := sums[f.StepID]
			if !ok {
				sum = &sentSkipped{}
				sums[f.StepID] = sum
				order = append(order, f.StepID)
			}
			sum.add(sent[versionStep{versionID: versionOf[f.Version], stepID: f.StepID}])
		}
	}
	report.Steps = lo.Map(order, func(stepID automationstep.ID, _ int) StepFigures {
		sum := sums[stepID]
		return StepFigures{
			StepID: stepID, Sent: sum.sent, Skipped: sum.skipped, SingleZone: sum.singleZone,
			SkippedByReason: skipreason.Counts(sum.skippedBy), WaitingByCause: deferralcause.Counts(sum.waiting),
		}
	})
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

// versionPath names a path of a fork of one version: the version's row id, the fork's Step id and
// the path's id.
type versionPath struct {
	versionID      int64
	stepID, pathID automationstep.ID
}

// tookAndGoal is how many Enrollments took a path and how many of them reached the goal.
type tookAndGoal struct{ took, goal int }

// pathsTaken counts the Automation's Enrollments by the version they began on and each path a
// fork sent them down, as their Step records say (ADR 0124), and those of them that reached the
// goal.
func pathsTaken(ctx context.Context, s *ent.Scoped, id int64) (map[versionPath]*tookAndGoal, error) {
	var rows []struct {
		VersionID int64                `sql:"version_id"`
		StepID    automationstep.ID    `sql:"step_id"`
		PathID    automationstep.ID    `sql:"path_id"`
		Status    automationrun.Status `sql:"status"`
		N         int                  `sql:"n"`
	}
	err := s.AutomationStepRecord().Query().
		Where(automationsteprecord.OutcomeEQ(automationsteprecord.OutcomePath)).
		Modify(func(sel *sql.Selector) {
			run := sql.Table(automationrun.Table)
			// The run's foreign key is composite: joined on both columns, the runs stay in the
			// scope's Workspace and the read takes the runs' Workspace and Automation index.
			sel.Join(run).OnP(sql.And(
				sql.ColumnsEQ(sel.C(automationsteprecord.FieldWorkspaceID), run.C(automationrun.FieldWorkspaceID)),
				sql.ColumnsEQ(sel.C(automationsteprecord.FieldRunID), run.C(automationrun.FieldID)),
			))
			sel.Where(sql.EQ(run.C(automationrun.FieldAutomationID), id))
			sel.Select(
				sql.As(run.C(automationrun.FieldVersionID), "version_id"),
				sql.As(sel.C(automationsteprecord.FieldStepID), "step_id"),
				sql.As(sel.C(automationsteprecord.FieldPathID), "path_id"),
				sql.As(run.C(automationrun.FieldStatus), "status"),
				sql.As(sql.Count("*"), "n"),
			).GroupBy(
				run.C(automationrun.FieldVersionID), sel.C(automationsteprecord.FieldStepID),
				sel.C(automationsteprecord.FieldPathID), run.C(automationrun.FieldStatus),
			)
		}).Scan(ctx, &rows)
	if err != nil {
		return nil, err
	}
	taken := map[versionPath]*tookAndGoal{}
	for _, r := range rows {
		k := versionPath{versionID: r.VersionID, stepID: r.StepID, pathID: r.PathID}
		c := taken[k]
		if c == nil {
			c = &tookAndGoal{}
			taken[k] = c
		}
		c.took += r.N
		if r.Status == automationrun.StatusGoalReached {
			c.goal += r.N
		}
	}
	return taken, nil
}

// pathFiguresOf sums what each path's Enrollments did by the fork's Step id and the path's id, in
// the order of the newest version that had each path: the fork's place there, then the path's
// place in the fork, a path the version no longer holds after the ones it does.
func pathFiguresOf(ctx context.Context, s *ent.Scoped, id int64, taken map[versionPath]*tookAndGoal) ([]PathFigures, error) {
	versionIDs := lo.Uniq(lo.Map(lo.Keys(taken), func(k versionPath, _ int) int64 { return k.versionID }))
	versions, err := s.AutomationVersion().Query().
		Where(automationversion.AutomationID(id), automationversion.IDIn(versionIDs...)).
		Order(ent.Desc(automationversion.FieldVersion)).
		All(ctx)
	if err != nil {
		return nil, err
	}
	figures := []PathFigures{}
	sums := map[forkPath]int{}
	for _, v := range versions {
		place := pathPlaces(v.Steps)
		var own []versionPath
		for k := range taken {
			if k.versionID == v.ID {
				own = append(own, k)
			}
		}
		slices.SortFunc(own, func(a, b versionPath) int {
			return cmp.Or(
				cmp.Compare(placeOfPath(place, a), placeOfPath(place, b)),
				cmp.Compare(a.stepID, b.stepID), cmp.Compare(a.pathID, b.pathID),
			)
		})
		for _, k := range own {
			fp := forkPath{stepID: k.stepID, pathID: k.pathID}
			i, ok := sums[fp]
			if !ok {
				i = len(figures)
				sums[fp] = i
				figures = append(figures, PathFigures{StepID: k.stepID, PathID: k.pathID})
			}
			figures[i].Enrollments += taken[k].took
			figures[i].GoalReached += taken[k].goal
		}
	}
	return figures, nil
}

// forkPath names a path of a fork, the same in every version that has it.
type forkPath struct{ stepID, pathID automationstep.ID }

// pathPlaces is the place of each path of each fork of the root path: the forks in pre-order and,
// within a fork, its paths in order.
func pathPlaces(root automationstep.Path) map[forkPath]int {
	place := map[forkPath]int{}
	for _, st := range automationstep.All(root) {
		for _, p := range automationstep.Forks(st) {
			place[forkPath{stepID: st.ID, pathID: p.ID}] = len(place)
		}
	}
	return place
}

// placeOfPath is the path's place in a version, after every path it holds when it holds none of
// that fork and id.
func placeOfPath(place map[forkPath]int, k versionPath) int {
	if pos, ok := place[forkPath{stepID: k.stepID, pathID: k.pathID}]; ok {
		return pos
	}
	return len(place)
}
