// Package abusedetection is the Enterprise abuse detector's pass (ADR 0026, ADR 0102): every
// hour it reads each Workspace's standing through the abuse watch (ee/domain/insights/abusedetector) and
// acts on two tiers. The lower one only warns the owners by email and marks the episode on the
// Workspace; the higher one freezes the Workspace through the core suspension (actor `system`)
// and tells them. It never lifts a suspension: only a Staff member does, and unsuspending stamps
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
	"github.com/getsphericon/sphericon/internal/domain/identity/memberships"
	"github.com/getsphericon/sphericon/internal/domain/identity/suspension"
	"github.com/getsphericon/sphericon/internal/domain/identity/workspaces"
	"github.com/getsphericon/sphericon/internal/kernel/i18n"
)

// Interval is how often the Edition runs the detector (Run).
const Interval = time.Hour

// Config is what a Detector is built from.
type Config struct {
	Limits abusedetector.Limits
	// SupportEmail is the contact the notices give an owner to appeal to; empty gives none.
	SupportEmail string
}

// Detector performs the detector's pass (ADR 0026). It is the raw-client entry of the
// control: like a job entry point it lists Workspaces, then scopes to each one.
type Detector struct {
	*abusedetector.Watch

	client  *ent.Client
	notify  memberships.Notify
	suspend *suspension.Suspender
	support string
}

// New builds the Detector. notify tells the owners through the platform's (system) sender, nil
// for none: the notices never go through a Workspace's own Integration, which a freeze holds. A
// freeze suspends through suspender, which tells the owners the same way.
func New(client *ent.Client, lic *licensekey.License, notify memberships.Notify, suspender *suspension.Suspender, cfg Config) *Detector {
	return &Detector{Watch: abusedetector.NewWatch(lic, cfg.Limits), client: client, notify: notify, suspend: suspender, support: cfg.SupportEmail}
}

// Summary is who a pass warned and who it froze.
type Summary struct {
	Warned, Frozen []int64
}

// Run is one pass at now over every Workspace. Unlicensed it does nothing. Per Workspace,
// in this order: a suspended one is skipped (its first actor and reason stand), an exempt
// one is skipped, then the Workspace is judged over the window since its last clearance.
// A freeze suspends it as `system`; a warning mails the owners once per episode, and
// leaving the warning tier ends the episode. It never lifts a suspension. A failed notice
// never undoes a freeze: it is logged, and a warning whose mail failed is retried next
// pass. One Workspace's failure does not stop the others; the errors are joined.
func (d *Detector) Run(ctx context.Context, now time.Time) (Summary, error) {
	var sum Summary
	if !d.Licensed() {
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
		breach, err := d.Standing(ctx, d.client.Scoped(ws.ID), ws, now)
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

// freeze suspends the Workspace as the system and tells its owners. Freezing is itself
// the escalation of a warning, so no warning mail goes out and the episode ends. A failed
// notice is logged, not returned: the freeze stands and nothing retries a suspended
// Workspace.
func (d *Detector) freeze(ctx context.Context, ws *ent.Workspace, b abusedetector.Breach) (bool, error) {
	changed, err := d.suspend.Suspend(ctx, ws.ID, suspension.System, b.Describe()+" (freeze)")
	if errors.Is(err, suspension.ErrNoticeNotSent) {
		slog.ErrorContext(ctx, "workspace frozen by the abuse detector, but the owner notice could not be sent", "workspace_id", ws.ID, "error", err)
	} else if err != nil || !changed {
		return false, err
	}
	if err := workspaces.ClearAbuseWarning(ctx, d.client, ws.ID); err != nil {
		slog.ErrorContext(ctx, "workspace frozen, but its warning episode could not be ended", "workspace_id", ws.ID, "error", err)
	}
	return true, nil
}

// metricNames words a breached rate in the warning to the owners.
var metricNames = map[abusedetector.Metric]i18n.ID{abusedetector.Complaint: i18n.MetricComplaint, abusedetector.Bounce: i18n.MetricBounce}

// warn mails the owners once per episode: the marker is set only after the mail went, so
// a failed send is retried, and is cleared when the Workspace leaves the tier.
func (d *Detector) warn(ctx context.Context, ws *ent.Workspace, b abusedetector.Breach, now time.Time) (bool, error) {
	if ws.AbuseWarningNotifiedAt != nil || d.notify == nil {
		return false, nil
	}
	data := map[string]any{
		"Workspace": ws.Name, "Domain": b.Domain, "Metric": i18n.T(metricNames[b.Metric], nil),
		"Rate": abusedetector.Percent(b.Rate()), "Count": b.Numerator, "Total": b.Denominator,
		"Limit": abusedetector.Percent(float64(b.LimitBP) / 10000), "Support": d.support,
	}
	if err := d.notify.Tell(ctx, ws.ID, memberships.Notice{Subject: i18n.EmailAbuseWarningSubject, Body: i18n.EmailAbuseWarningBody}, data); err != nil {
		return false, fmt.Errorf("warn workspace %d: %w", ws.ID, err)
	}
	if err := workspaces.MarkAbuseWarned(ctx, d.client, ws.ID, now); err != nil {
		return false, fmt.Errorf("record the warning of workspace %d: %w", ws.ID, err)
	}
	return true, nil
}
