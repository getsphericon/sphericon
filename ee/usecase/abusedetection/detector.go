// Package abusedetection is the Enterprise abuse detector's pass (ADR 0026, ADR 0102): every
// hour it reads each Workspace's standing through the abuse watch (ee/domain/insights/abusedetector) and
// acts on two tiers. The lower one only warns the owners and marks the episode on the Workspace;
// the higher one freezes the Workspace through the core suspension (actor `system`). Each
// publishes its event in the transaction of its change, and the Notices tell the owners (ADR
// 0105), through the platform's System sender, never through a Workspace's own Integration, which
// a freeze holds. It never lifts a suspension: only a Staff member does, and unsuspending stamps
// cleared_at, after which only newer Events count.
//
// Governed by ee/LICENSE, not the AGPL.
package abusedetection

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/getsphericon/sphericon/ee/domain/insights/abusedetector"
	"github.com/getsphericon/sphericon/ee/licensekey"
	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/workspace"
	"github.com/getsphericon/sphericon/internal/domain/identity/suspension"
	"github.com/getsphericon/sphericon/internal/domain/identity/workspaces"
	"github.com/getsphericon/sphericon/internal/infra/events"
	"github.com/getsphericon/sphericon/internal/kernel/i18n"
)

// Interval is how often the Edition runs the detector (Run).
const Interval = time.Hour

// Config is what the detector is built from.
type Config struct {
	Limits abusedetector.Limits
}

// UseCase performs the detector's pass (ADR 0026). It is the raw-client entry of the
// control: like a job entry point it lists Workspaces, then scopes to each one.
type UseCase struct {
	watch   *abusedetector.Watch
	client  *ent.Client
	bus     *events.Bus
	suspend *suspension.Suspender
	now     func() time.Time
}

// New builds the use case. A warning is recorded and published through bus; a freeze suspends
// through suspender. now is the Instance clock a pass judges at.
func New(client *ent.Client, bus *events.Bus, lic *licensekey.License, suspender *suspension.Suspender, cfg Config, now func() time.Time) *UseCase {
	return &UseCase{
		watch: abusedetector.NewWatch(lic, cfg.Limits), client: client, bus: bus, suspend: suspender, now: now,
	}
}

// Summary is who a pass warned and who it froze.
type Summary struct {
	Warned, Frozen []int64
}

// Run is one pass over every Workspace, at the clock's now. Unlicensed it does nothing. Per Workspace,
// in this order: a suspended one is skipped (its first actor and reason stand), an exempt
// one is skipped, then the Workspace is judged over the window since its last clearance.
// A freeze suspends it as `system`; a warning is published once per episode, and leaving the
// warning tier ends the episode. It never lifts a suspension. The owners are told by the Notices
// of what it published, which retry their own sends. One Workspace's failure does not stop the
// others; the errors are joined.
func (d *UseCase) Run(ctx context.Context) (Summary, error) {
	now := d.now()
	var sum Summary
	if !d.watch.Licensed() {
		return sum, nil
	}
	// A Workspace that was warned and then suspended by hand or exempted has left the
	// detector's reach: end its episode, so a stale marker neither lists it as warned nor
	// mutes the warning it earns after being cleared.
	var errs []error
	if err := workspaces.ClearAbuseWarnings(ctx, d.client, workspace.Or(suspension.Suspended(), workspace.AbuseExempt(true))); err != nil {
		errs = append(errs, fmt.Errorf("end the warning episodes of suspended and exempt workspaces: %w", err))
	}
	candidates, err := d.client.Workspace.Query().
		Where(suspension.NotSuspended(), workspace.AbuseExempt(false), workspaces.Live()).
		All(ctx)
	if err != nil {
		return sum, errors.Join(append(errs, fmt.Errorf("list workspaces for the abuse detector: %w", err))...)
	}
	for _, ws := range candidates {
		breach, err := d.watch.Standing(ctx, d.client.Scoped(ws.ID), ws, now)
		if err != nil {
			errs = append(errs, fmt.Errorf("rates of workspace %d: %w", ws.ID, err))
			continue
		}
		switch breach.Tier {
		case abusedetector.TierFreeze:
			frozen, err := d.freeze(ctx, ws, breach)
			if frozen {
				sum.Frozen = append(sum.Frozen, ws.ID)
			}
			if err != nil {
				errs = append(errs, err)
			}
		case abusedetector.TierWarn:
			warned, err := d.warn(ctx, ws, breach, now)
			if warned {
				sum.Warned = append(sum.Warned, ws.ID)
			}
			if err != nil {
				errs = append(errs, err)
			}
		default:
			if ws.AbuseWarningNotifiedAt != nil {
				if err := workspaces.ClearAbuseWarning(ctx, d.client, ws.ID); err != nil {
					errs = append(errs, fmt.Errorf("end the warning episode of workspace %d: %w", ws.ID, err))
				}
			}
		}
	}
	return sum, errors.Join(errs...)
}

// freeze suspends the Workspace as the system, which tells its owners. Freezing is itself
// the escalation of a warning, so no warning goes out and the episode ends.
func (d *UseCase) freeze(ctx context.Context, ws *ent.Workspace, b abusedetector.Breach) (bool, error) {
	changed, err := d.suspend.Suspend(ctx, ws.ID, suspension.System, b.Describe()+" (freeze)")
	if err != nil || !changed {
		return false, err
	}
	if err := workspaces.ClearAbuseWarning(ctx, d.client, ws.ID); err != nil {
		slog.ErrorContext(ctx, "workspace frozen, but its warning episode could not be ended", "workspace_id", ws.ID, "error", err)
	}
	return true, nil
}

// metricNames words a breached rate in the warning to the owners.
var metricNames = map[abusedetector.Metric]i18n.ID{abusedetector.Complaint: i18n.MetricComplaint, abusedetector.Bounce: i18n.MetricBounce}

// warn warns the owners once per episode: the marker is set and the warning published in one
// transaction, so the warning exists iff the marker does, and the marker is cleared when the
// Workspace leaves the tier.
func (d *UseCase) warn(ctx context.Context, ws *ent.Workspace, b abusedetector.Breach, now time.Time) (bool, error) {
	if ws.AbuseWarningNotifiedAt != nil {
		return false, nil
	}
	err := d.bus.WithinTx(ctx, func(tx *ent.Client, pub events.Publisher) error {
		if err := workspaces.MarkAbuseWarned(ctx, tx, ws.ID, now); err != nil {
			return fmt.Errorf("record the warning of workspace %d: %w", ws.ID, err)
		}
		return pub.Publish(ctx, &events.AbuseWarned{
			WorkspaceID: ws.ID, Domain: b.Domain, Metric: i18n.T(metricNames[b.Metric], nil),
			Rate: abusedetector.Percent(b.Rate()), Limit: abusedetector.Percent(float64(b.LimitBP) / 10000),
			Count: int64(b.Numerator), Total: int64(b.Denominator),
		})
	})
	if err != nil {
		return false, fmt.Errorf("warn workspace %d: %w", ws.ID, err)
	}
	return true, nil
}
