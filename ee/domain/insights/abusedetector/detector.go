package abusedetector

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/samber/lo"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/predicate"
	"github.com/getsphericon/sphericon/ent/workspace"
	"github.com/getsphericon/sphericon/internal/domain/identity/suspension"
	"github.com/getsphericon/sphericon/internal/domain/identity/workspaces"
	"github.com/getsphericon/sphericon/internal/infra/events"
	"github.com/getsphericon/sphericon/internal/kernel/i18n"
)

// Interval is how often the Edition runs the detector over every Workspace (Detector.Due,
// Detector.Judge).
const Interval = time.Hour

// Detector performs the Abuse detector's hourly detection (ADR 0026, GLOSSARY "Abuse detector"):
// it reads each Workspace's standing through its Watch and acts on two tiers. The lower one only
// warns the owners and marks the episode on the Workspace; the higher one freezes the Workspace
// through the core suspension (actor `system`). Each publishes its event in the transaction of its
// change, and the Notices tell the owners (ADR 0105), through the platform's System sender, never
// through a Workspace's own Integration, which a freeze holds. It never lifts a suspension: only a
// Staff member does, and unsuspending stamps cleared_at, after which only newer Events count.
//
// It holds the raw client: like a job entry point it lists the Workspaces, and a pass scopes to the
// one it judges after reading its row.
type Detector struct {
	watch      *Watch
	client     *ent.Client
	bus        *events.Bus
	suspend    *suspension.Suspender
	workspaces *workspaces.Module
}

// NewDetector builds the detector over the watch, whose license, limits and clock a pass judges
// by. A warning is recorded on the Workspace's standing through ws and published through bus; a
// freeze suspends through suspender.
func NewDetector(client *ent.Client, bus *events.Bus, watch *Watch, suspender *suspension.Suspender, ws *workspaces.Module) *Detector {
	return &Detector{watch: watch, client: client, bus: bus, suspend: suspender, workspaces: ws}
}

// Outcome is what judging one Workspace did to it.
type Outcome int

const (
	// Unchanged is a Workspace neither warned nor frozen by this pass.
	Unchanged Outcome = iota
	// Warned is a Workspace whose owners this pass warned.
	Warned
	// Frozen is a Workspace this pass suspended.
	Frozen
)

// inReach matches the Workspaces the detector judges: live, not suspended and not exempt.
func inReach() predicate.Workspace {
	return workspace.And(suspension.NotSuspended(), workspace.AbuseExempt(false), workspaces.Live())
}

// outOfReach matches the Workspaces that were warned and have since been suspended by hand or
// exempted: they have left the detector's reach, and their episode ends.
func outOfReach() predicate.Workspace {
	return workspace.And(workspaces.AbuseWarned(), workspace.Or(suspension.Suspended(), workspace.AbuseExempt(true)))
}

// Due is the Workspaces a pass judges one by one (Judge): the ones in its reach, and the warned ones out
// of the detector's reach, whose episode a pass ends. Unlicensed it is none.
func (d *Detector) Due(ctx context.Context) ([]int64, error) {
	if !d.watch.Licensed() {
		return nil, nil
	}
	ids, err := d.client.Workspace.Query().
		Where(workspace.Or(inReach(), outOfReach())).
		Order(ent.Asc(workspace.FieldID)).
		IDs(ctx)
	if err != nil {
		return nil, fmt.Errorf("list workspaces for the abuse detector: %w", err)
	}
	return ids, nil
}

// Judge is one pass over the Workspace, at the watch's clock's now. Unlicensed it does nothing.
// A Workspace that was warned and then suspended by hand or exempted has left the detector's
// reach: its episode ends, so a stale marker neither lists it as warned nor mutes the warning it
// earns after being cleared. Any other suspended, exempt or deleted Workspace is skipped (a
// suspended one keeps its first actor and reason). The rest is judged over the window since its
// last clearance: a freeze suspends it as `system`; a warning is published once per episode, and
// leaving the warning tier ends the episode. It never lifts a suspension. The owners are told by
// the Notices of what it published, which retry their own sends.
func (d *Detector) Judge(ctx context.Context, workspaceID int64) (Outcome, error) {
	if !d.watch.Licensed() {
		return Unchanged, nil
	}
	now := d.watch.now()
	ws, err := d.client.Workspace.Query().
		Where(workspace.ID(workspaceID), inReach()).
		WithStanding().
		Only(ctx)
	if ent.IsNotFound(err) {
		if err := d.workspaces.ClearAbuseWarnings(ctx, d.client, workspace.ID(workspaceID), outOfReach()); err != nil {
			return Unchanged, fmt.Errorf("end the warning episode of workspace %d: %w", workspaceID, err)
		}
		return Unchanged, nil
	}
	if err != nil {
		return Unchanged, fmt.Errorf("read workspace %d for the abuse detector: %w", workspaceID, err)
	}
	breach, err := d.watch.standing(ctx, d.client.Scoped(ws.ID), ws, now)
	if err != nil {
		return Unchanged, fmt.Errorf("rates of workspace %d: %w", ws.ID, err)
	}
	switch breach.Tier {
	case TierFreeze:
		frozen, err := d.freeze(ctx, ws, breach)
		return lo.Ternary(frozen, Frozen, Unchanged), err
	case TierWarn:
		warned, err := d.warn(ctx, ws, breach)
		return lo.Ternary(warned, Warned, Unchanged), err
	default:
		if workspaces.StandingOf(ws).AbuseWarningNotifiedAt != nil {
			if err := d.workspaces.ClearAbuseWarning(ctx, d.client, ws.ID); err != nil {
				return Unchanged, fmt.Errorf("end the warning episode of workspace %d: %w", ws.ID, err)
			}
		}
		return Unchanged, nil
	}
}

// freeze suspends the Workspace as the system, which tells its owners. Freezing is itself
// the escalation of a warning, so no warning goes out and the episode ends.
func (d *Detector) freeze(ctx context.Context, ws *ent.Workspace, b Breach) (bool, error) {
	changed, err := d.suspend.Suspend(ctx, ws.ID, suspension.SuspendInput{By: suspension.System, Reason: b.Describe() + " (freeze)"})
	if err != nil || !changed {
		return false, err
	}
	if err := d.workspaces.ClearAbuseWarning(ctx, d.client, ws.ID); err != nil {
		slog.ErrorContext(ctx, "workspace frozen, but its warning episode could not be ended", "workspace_id", ws.ID, "error", err)
	}
	return true, nil
}

// metricNames words a breached rate in the warning to the owners.
var metricNames = map[Metric]i18n.ID{Complaint: i18n.MetricComplaint, Bounce: i18n.MetricBounce}

// warn warns the owners once per episode: the marker is set and the warning published in one
// transaction, so the warning exists iff the marker does, and the marker is cleared when the
// Workspace leaves the tier.
func (d *Detector) warn(ctx context.Context, ws *ent.Workspace, b Breach) (bool, error) {
	if workspaces.StandingOf(ws).AbuseWarningNotifiedAt != nil {
		return false, nil
	}
	err := d.bus.WithinTx(ctx, func(tx *ent.Client, pub events.Publisher) error {
		if err := d.workspaces.MarkAbuseWarned(ctx, tx, ws.ID); err != nil {
			return fmt.Errorf("record the warning of workspace %d: %w", ws.ID, err)
		}
		return pub.Publish(ctx, &events.AbuseWarned{
			WorkspaceID: ws.ID, Domain: b.Domain, Metric: i18n.T(metricNames[b.Metric], nil),
			Rate: Percent(b.Rate()), Limit: Percent(float64(b.LimitBP) / 10000),
			Count: int64(b.Numerator), Total: int64(b.Denominator),
		})
	})
	if err != nil {
		return false, fmt.Errorf("warn workspace %d: %w", ws.ID, err)
	}
	return true, nil
}
